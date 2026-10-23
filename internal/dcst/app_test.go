package dcst

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseResultFileKeepsTop20(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.csv")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	_ = writer.Write([]string{"IP 地址", "已发送", "已接收", "丢包率", "平均延迟", "下载速度(MB/s)", "地区码"})
	for i := 0; i < 25; i++ {
		_ = writer.Write([]string{
			"104.16.0." + strconv.Itoa(i+1),
			"4", "4", "0.00",
			strconv.Itoa(100 - i),
			strconv.FormatFloat(float64(i), 'f', 2, 64),
			"HKG",
		})
	}
	writer.Flush()
	_ = file.Close()

	results, candidates, err := parseResultFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if candidates != 25 {
		t.Fatalf("candidates = %d, want 25", candidates)
	}
	if len(results) != ResultLimit {
		t.Fatalf("results = %d, want %d", len(results), ResultLimit)
	}
	for i := 1; i < len(results); i++ {
		if results[i].DownloadMBs > results[i-1].DownloadMBs {
			t.Fatalf("results are not sorted at %d", i)
		}
	}
	if results[0].DownloadMBs != 24 {
		t.Fatalf("best speed = %v, want 24", results[0].DownloadMBs)
	}
}

func TestExecuteRunInvokesEngineAndPersistsTop20(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	t.Setenv("RUN_ON_START", "false")
	t.Setenv("CLOUDFLARE_IPS_URL", "http://127.0.0.1:1/unavailable")

	tempDir := t.TempDir()
	bundleDir := filepath.Join(tempDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "ip.txt"), []byte("104.16.0.0/12\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUNDLED_IP_DIR", bundleDir)

	enginePath := filepath.Join(tempDir, "fake-engine")
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	script.WriteString("out=\"\"\n")
	script.WriteString("while [ $# -gt 0 ]; do\n")
	script.WriteString("  if [ \"$1\" = \"-o\" ]; then out=\"$2\"; shift 2; else shift; fi\n")
	script.WriteString("done\n")
	script.WriteString("printf 'IP 地址,已发送,已接收,丢包率,平均延迟,下载速度(MB/s),地区码\\n' > \"$out\"\n")
	for i := 1; i <= 25; i++ {
		script.WriteString(fmt.Sprintf("printf '104.16.0.%d,4,4,0.00,25.00,%d.00,HKG\\n' >> \"$out\"\n", i, i))
	}
	if err := os.WriteFile(enginePath, []byte(script.String()), 0o755); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(filepath.Join(tempDir, "data"), enginePath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	cfg := app.Config()
	cfg.Candidates = 25
	cfg.UseIPv6 = false
	if err := app.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := app.executeRun(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := app.Snapshot()
	if snapshot.Count != ResultLimit || len(snapshot.Results) != ResultLimit {
		t.Fatalf("count = %d, want %d", snapshot.Count, ResultLimit)
	}
	if snapshot.Results[0].DownloadMBs != 25 {
		t.Fatalf("best speed = %v, want 25", snapshot.Results[0].DownloadMBs)
	}
}

func TestConfigNormalizeRejectsInvalidCandidates(t *testing.T) {
	cfg := defaultConfig()
	cfg.IntervalMinutes = 60
	cfg.Candidates = ResultLimit - 1
	if err := cfg.normalize(); err == nil {
		t.Fatal("expected invalid candidate count")
	}
	cfg.Candidates = 40
	if err := cfg.normalize(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestResultAPIAndAdminAuth(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	t.Setenv("RUN_ON_START", "false")
	app, err := NewApp(t.TempDir(), "/bin/true", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	app.state = runnerState{
		Ready:      true,
		UpdatedAt:  &now,
		LastRunAt:  &now,
		Candidates: 40,
		Results: []IPResult{{
			IP: "104.16.1.1", Sent: 4, Received: 4, DownloadMBs: 12.5, LatencyMS: 25, Region: "HKG",
		}},
	}
	handler := NewHandler(app)

	resultReq := httptest.NewRequest(http.MethodGet, "/result", nil)
	resultRec := httptest.NewRecorder()
	handler.ServeHTTP(resultRec, resultReq)
	if resultRec.Code != http.StatusOK {
		t.Fatalf("result status = %d", resultRec.Code)
	}
	if resultRec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS header")
	}
	var snapshot Snapshot
	if err := json.Unmarshal(resultRec.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Count != 1 || snapshot.Results[0].IP != "104.16.1.1" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/auth", bytes.NewReader(nil))
	unauthorizedRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedRec, unauthorized)
	if unauthorizedRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorizedRec.Code)
	}

	authorized := httptest.NewRequest(http.MethodPost, "/api/auth", bytes.NewReader(nil))
	authorized.Header.Set("Authorization", "Bearer test-admin-token")
	authorizedRec := httptest.NewRecorder()
	handler.ServeHTTP(authorizedRec, authorized)
	if authorizedRec.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d", authorizedRec.Code)
	}
}

func TestPasswordSetupLoginAndChange(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("RUN_ON_START", "false")
	dataDir := t.TempDir()
	app, err := NewApp(dataDir, "/bin/true", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(app)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("auth status = %d", statusRec.Code)
	}
	var status AuthStatus
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Initialized {
		t.Fatal("new app should require password setup")
	}

	setupBody := bytes.NewBufferString(`{"password":"12345670","confirm":"12345670"}`)
	setupReq := httptest.NewRequest(http.MethodPost, "/api/setup", setupBody)
	setupReq.RemoteAddr = "127.0.0.1:10000"
	setupRec := httptest.NewRecorder()
	handler.ServeHTTP(setupRec, setupReq)
	if setupRec.Code != http.StatusNoContent {
		t.Fatalf("setup status = %d body=%s", setupRec.Code, setupRec.Body.String())
	}
	authFile, err := os.ReadFile(filepath.Join(dataDir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(authFile, []byte("12345670")) {
		t.Fatal("auth file must not contain the plaintext password")
	}
	cookies := setupRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("setup did not set session cookie")
	}

	loginBody := bytes.NewBufferString(`{"password":"12345670"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", loginBody)
	loginReq.RemoteAddr = "127.0.0.1:10001"
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusNoContent {
		t.Fatalf("login status = %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	loginCookies := loginRec.Result().Cookies()
	if len(loginCookies) == 0 {
		t.Fatal("login did not set session cookie")
	}

	changeBody := bytes.NewBufferString(`{"current":"12345670","password":"76543210","confirm":"76543210"}`)
	changeReq := httptest.NewRequest(http.MethodPost, "/api/password", changeBody)
	changeReq.RemoteAddr = "127.0.0.1:10002"
	changeReq.AddCookie(loginCookies[0])
	changeRec := httptest.NewRecorder()
	handler.ServeHTTP(changeRec, changeReq)
	if changeRec.Code != http.StatusNoContent {
		t.Fatalf("change password status = %d body=%s", changeRec.Code, changeRec.Body.String())
	}

	statusReq = httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusReq.AddCookie(changeRec.Result().Cookies()[0])
	statusRec = httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	status = AuthStatus{}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Initialized || !status.Authenticated {
		t.Fatalf("unexpected auth status after password change: %+v", status)
	}

	restarted, err := NewApp(dataDir, "/bin/true", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.PasswordConfigured() {
		t.Fatal("password should persist across app restart")
	}
	if _, err := restarted.Login("76543210", "127.0.0.1"); err != nil {
		t.Fatalf("login after restart failed: %v", err)
	}
}

func TestPasswordValidation(t *testing.T) {
	for _, password := range []string{"1234567", "123456789", "1234567a", "88888888", "12345678"} {
		if err := validatePassword(password); err == nil {
			t.Fatalf("password %q should be rejected", password)
		}
	}
	if err := validatePassword("48261937"); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
}

func TestRequestUpdateCallsInternalUpdater(t *testing.T) {
	var gotAuthorization string
	updater := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	defer updater.Close()

	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("RUN_ON_START", "false")
	t.Setenv("UPDATE_ENDPOINT", updater.URL)
	t.Setenv("UPDATE_TOKEN", "updater-token")
	app, err := NewApp(t.TempDir(), "/bin/true", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RequestUpdate(); err != nil {
		t.Fatal(err)
	}
	if gotAuthorization != "Bearer updater-token" {
		t.Fatalf("authorization = %q", gotAuthorization)
	}
	if !app.Snapshot().UpdateEnabled {
		t.Fatal("snapshot should report update support")
	}
}
