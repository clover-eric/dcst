package dcst

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed web
var webFiles embed.FS

type Server struct {
	app *App
}

func NewHandler(app *App) http.Handler {
	server := &Server{app: app}
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.index)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(mustSubFS(webFiles, "web")))))
	mux.HandleFunc("/health", server.health)
	mux.HandleFunc("/api/status", server.status)
	mux.HandleFunc("/api/config", server.config)
	mux.HandleFunc("/api/auth/status", server.authStatus)
	mux.HandleFunc("/api/auth", server.auth)
	mux.HandleFunc("/api/setup", server.setup)
	mux.HandleFunc("/api/login", server.login)
	mux.HandleFunc("/api/logout", server.logout)
	mux.HandleFunc("/api/password", server.password)
	mux.HandleFunc("/api/run", server.run)
	mux.HandleFunc("/api/update", server.update)
	mux.HandleFunc("/result", server.result)
	mux.HandleFunc("/api/result", server.result)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "web interface unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Snapshot())
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.app.Config())
	case http.MethodPost:
		if !s.requireAdmin(w, r) {
			return
		}
		defer r.Body.Close()
		var cfg Config
		decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.app.SetConfig(cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.app.Config())
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) auth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, s.app.AuthStatus(r))
}

type passwordRequest struct {
	Password string `json:"password"`
	Confirm  string `json:"confirm"`
	Current  string `json:"current"`
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var payload passwordRequest
	if !decodePasswordRequest(w, r, &payload) {
		return
	}
	token, err := s.app.SetupPassword(payload.Password, payload.Confirm)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrAlreadyInitialized) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	s.app.SetSessionCookie(w, r, token)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var payload passwordRequest
	if !decodePasswordRequest(w, r, &payload) {
		return
	}
	token, err := s.app.Login(payload.Password, clientIP(r))
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, ErrNotInitialized) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	s.app.SetSessionCookie(w, r, token)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	s.app.Logout(r)
	s.app.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	var payload passwordRequest
	if !decodePasswordRequest(w, r, &payload) {
		return
	}
	token, err := s.app.ChangePassword(payload.Current, payload.Password, payload.Confirm)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrInvalidPassword) {
			status = http.StatusUnauthorized
		}
		writeError(w, status, err.Error())
		return
	}
	s.app.SetSessionCookie(w, r, token)
	w.WriteHeader(http.StatusNoContent)
}

func decodePasswordRequest(w http.ResponseWriter, r *http.Request, target *passwordRequest) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	if err := s.app.RunNow(); err != nil {
		if errors.Is(err, ErrAlreadyRunning) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"message":  "测速任务已启动",
	})
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	if err := s.app.RequestUpdate(); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrUpdateUnavailable) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"message":  "更新已触发，容器可能在十几秒后自动重启",
	})
}

func (s *Server) result(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodOptions)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	snapshot := s.app.Snapshot()
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))) {
	case "csv":
		writeResultCSV(w, snapshot)
	case "text", "plain":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, result := range snapshot.Results {
			_, _ = fmt.Fprintln(w, result.IP)
		}
	default:
		writeJSON(w, http.StatusOK, snapshot)
	}
}

func writeResultCSV(w http.ResponseWriter, snapshot Snapshot) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="dcst-result.csv"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"IP 地址", "已发送", "已接收", "丢包率", "平均延迟", "下载速度(MB/s)", "地区码", "更新时间"})
	updated := ""
	if snapshot.UpdatedAt != nil {
		updated = snapshot.UpdatedAt.Format(time.RFC3339)
	}
	for _, result := range snapshot.Results {
		_ = writer.Write([]string{
			result.IP,
			strconv.Itoa(result.Sent),
			strconv.Itoa(result.Received),
			strconv.FormatFloat(result.LossRate, 'f', 2, 64),
			strconv.FormatFloat(result.LatencyMS, 'f', 2, 64),
			strconv.FormatFloat(result.DownloadMBs, 'f', 2, 64),
			result.Region,
			updated,
		})
	}
	writer.Flush()
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.app.Authenticated(r) {
		writeError(w, http.StatusUnauthorized, "管理登录已失效")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func mustSubFS(source fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(source, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
