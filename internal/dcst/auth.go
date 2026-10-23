package dcst

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "dcst_session"
	sessionTTL        = 24 * time.Hour
	passwordHashCost  = 12
	maxLoginFailures  = 5
	loginWindow       = 5 * time.Minute
)

var (
	ErrAlreadyInitialized = errors.New("管理密码已经初始化")
	ErrNotInitialized     = errors.New("请先设置初始化密码")
	ErrInvalidPassword    = errors.New("密码错误")
)

type passwordRecord struct {
	Version      int       `json:"version"`
	PasswordHash string    `json:"password_hash"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type loginAttempt struct {
	Failures    int
	WindowStart time.Time
	BlockedTill time.Time
}

func (a *App) loadPasswordRecord() error {
	data, err := os.ReadFile(a.authPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var record passwordRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("读取管理密码失败: %w", err)
	}
	if record.PasswordHash == "" {
		return errors.New("管理密码文件无效")
	}
	if _, err := bcrypt.Cost([]byte(record.PasswordHash)); err != nil {
		return fmt.Errorf("管理密码哈希无效: %w", err)
	}
	a.authMu.Lock()
	a.passwordHash = []byte(record.PasswordHash)
	a.passwordConfigured = true
	a.authMu.Unlock()
	return nil
}

func validatePassword(password string) error {
	if len(password) != 8 {
		return errors.New("登录密码必须正好是 8 位数字")
	}
	allSame := true
	for i := range password {
		if password[i] < '0' || password[i] > '9' {
			return errors.New("登录密码只能使用数字")
		}
		if password[i] != password[0] {
			allSame = false
		}
	}
	if allSame {
		return errors.New("不能使用 8 位完全相同的数字")
	}
	if password == "01234567" || password == "12345678" || password == "87654321" || password == "98765432" {
		return errors.New("不能使用明显的连续数字序列")
	}
	return nil
}

func (a *App) PasswordConfigured() bool {
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	return a.passwordConfigured
}

func (a *App) SetupPassword(password, confirm string) (string, error) {
	if password != confirm {
		return "", errors.New("两次输入的密码不一致")
	}
	if err := validatePassword(password); err != nil {
		return "", err
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.passwordConfigured {
		return "", ErrAlreadyInitialized
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		return "", err
	}
	record := passwordRecord{
		Version:      1,
		PasswordHash: string(hash),
		UpdatedAt:    time.Now(),
	}
	if err := saveJSONAtomic(a.authPath, record); err != nil {
		return "", err
	}
	a.passwordHash = hash
	a.passwordConfigured = true
	a.sessions = make(map[string]time.Time)
	return a.newSessionLocked()
}

func (a *App) Login(password, clientIP string) (string, error) {
	if !a.PasswordConfigured() {
		return "", ErrNotInitialized
	}
	now := time.Now()
	a.authMu.Lock()
	attempt := a.loginAttempts[clientIP]
	if attempt != nil {
		if now.Before(attempt.BlockedTill) {
			remaining := time.Until(attempt.BlockedTill).Round(time.Second)
			a.authMu.Unlock()
			return "", fmt.Errorf("登录尝试过多，请在 %s 后重试", remaining)
		}
		if now.Sub(attempt.WindowStart) > loginWindow {
			delete(a.loginAttempts, clientIP)
			attempt = nil
		}
	}
	hash := append([]byte(nil), a.passwordHash...)
	a.authMu.Unlock()

	if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil {
		a.authMu.Lock()
		if attempt == nil {
			attempt = &loginAttempt{WindowStart: now}
			a.loginAttempts[clientIP] = attempt
		}
		attempt.Failures++
		if attempt.Failures >= maxLoginFailures {
			attempt.BlockedTill = now.Add(loginWindow)
			attempt.Failures = 0
			attempt.WindowStart = now
		}
		a.authMu.Unlock()
		return "", ErrInvalidPassword
	}

	a.authMu.Lock()
	defer a.authMu.Unlock()
	delete(a.loginAttempts, clientIP)
	return a.newSessionLocked()
}

func (a *App) ChangePassword(current, next, confirm string) (string, error) {
	if current == "" {
		return "", errors.New("请输入当前密码")
	}
	if next != confirm {
		return "", errors.New("两次输入的新密码不一致")
	}
	if err := validatePassword(next); err != nil {
		return "", err
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if !a.passwordConfigured {
		return "", ErrNotInitialized
	}
	if err := bcrypt.CompareHashAndPassword(a.passwordHash, []byte(current)); err != nil {
		return "", ErrInvalidPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), passwordHashCost)
	if err != nil {
		return "", err
	}
	record := passwordRecord{
		Version:      1,
		PasswordHash: string(hash),
		UpdatedAt:    time.Now(),
	}
	if err := saveJSONAtomic(a.authPath, record); err != nil {
		return "", err
	}
	a.passwordHash = hash
	a.sessions = make(map[string]time.Time)
	return a.newSessionLocked()
}

func (a *App) Authenticated(r *http.Request) bool {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if a.validSession(cookie.Value) {
			return true
		}
	}
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	}
	if token == "" {
		return false
	}
	a.authMu.RLock()
	legacy := a.legacyAdminToken
	a.authMu.RUnlock()
	return legacy != "" && subtle.ConstantTimeCompare([]byte(token), []byte(legacy)) == 1
}

func (a *App) AuthStatus(r *http.Request) AuthStatus {
	return AuthStatus{
		Initialized:   a.PasswordConfigured(),
		Authenticated: a.Authenticated(r),
		PasswordRule:  "8 位纯数字",
	}
}

func (a *App) Logout(r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return
	}
	a.authMu.Lock()
	delete(a.sessions, cookie.Value)
	a.authMu.Unlock()
}

func (a *App) SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
		Expires:  time.Now().Add(sessionTTL),
	})
}

func (a *App) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
}

func (a *App) validSession(token string) bool {
	now := time.Now()
	a.authMu.Lock()
	defer a.authMu.Unlock()
	expiry, ok := a.sessions[token]
	if !ok {
		return false
	}
	if now.After(expiry) {
		delete(a.sessions, token)
		return false
	}
	return true
}

func (a *App) newSessionLocked() (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(randomBytes)
	now := time.Now()
	for key, expiry := range a.sessions {
		if now.After(expiry) {
			delete(a.sessions, key)
		}
	}
	a.sessions[token] = now.Add(sessionTTL)
	return token, nil
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	value := strings.TrimSpace(header)
	if strings.HasPrefix(value, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(value, prefix))
	}
	return value
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
