package dcst

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrAlreadyRunning = errors.New("已有测速任务正在运行")
)

type App struct {
	dataDir    string
	enginePath string
	logger     *slog.Logger

	configPath string
	resultPath string
	adminToken string
	runOnStart bool
	jobTimeout time.Duration

	configMu sync.RWMutex
	config   Config

	stateMu sync.RWMutex
	state   runnerState

	running atomic.Bool

	nextRunMu sync.RWMutex
	nextRunAt time.Time

	scheduleWake chan struct{}
}

func NewApp(dataDir, enginePath string, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "work"), 0o755); err != nil {
		return nil, err
	}
	app := &App{
		dataDir:      dataDir,
		enginePath:   enginePath,
		logger:       logger,
		configPath:   filepath.Join(dataDir, "config.json"),
		resultPath:   filepath.Join(dataDir, "result.json"),
		runOnStart:   envBool("RUN_ON_START", true),
		jobTimeout:   time.Duration(envInt("JOB_TIMEOUT_MINUTES", 30)) * time.Minute,
		scheduleWake: make(chan struct{}, 1),
	}
	if app.jobTimeout < 5*time.Minute {
		app.jobTimeout = 30 * time.Minute
	}

	cfg, err := loadConfig(app.configPath)
	if err != nil {
		logger.Warn("配置文件无效，使用环境变量默认值", "error", err)
		cfg = defaultConfig()
		if normalizeErr := cfg.normalize(); normalizeErr != nil {
			return nil, normalizeErr
		}
		if saveErr := saveJSONAtomic(app.configPath, cfg); saveErr != nil {
			return nil, saveErr
		}
	}
	app.config = cfg

	token, err := app.loadOrCreateAdminToken()
	if err != nil {
		return nil, err
	}
	app.adminToken = token

	app.state = runnerState{Results: []IPResult{}}
	if data, readErr := os.ReadFile(app.resultPath); readErr == nil {
		var stored runnerState
		if json.Unmarshal(data, &stored) == nil {
			if len(stored.Results) > ResultLimit {
				stored.Results = stored.Results[:ResultLimit]
			}
			if stored.Results == nil {
				stored.Results = []IPResult{}
			}
			app.state = stored
		}
	}
	return app, nil
}

func (a *App) Config() Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.config
}

func (a *App) SetConfig(cfg Config) error {
	if err := cfg.normalize(); err != nil {
		return err
	}
	if err := saveJSONAtomic(a.configPath, cfg); err != nil {
		return err
	}
	a.configMu.Lock()
	a.config = cfg
	a.configMu.Unlock()
	a.notifyScheduler()
	return nil
}

func (a *App) Snapshot() Snapshot {
	a.stateMu.RLock()
	state := a.state
	state.Results = append([]IPResult{}, state.Results...)
	a.stateMu.RUnlock()
	cfg := a.Config()
	a.nextRunMu.RLock()
	next := a.nextRunAt
	a.nextRunMu.RUnlock()
	var nextPtr *time.Time
	if !next.IsZero() {
		value := next
		nextPtr = &value
	}
	return Snapshot{
		Ready:       state.Ready,
		Running:     a.running.Load(),
		UpdatedAt:   state.UpdatedAt,
		LastRunAt:   state.LastRunAt,
		NextRunAt:   nextPtr,
		LastError:   state.LastError,
		IPSource:    state.IPSource,
		Count:       len(state.Results),
		Candidates:  state.Candidates,
		Results:     state.Results,
		AdminAuth:   true,
		Service:     ServiceName,
		Version:     BuildVersion,
		IntervalMin: cfg.IntervalMinutes,
	}
}

func (a *App) RunNow() error {
	if !a.running.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	go func() {
		defer func() {
			a.running.Store(false)
			a.notifyScheduler()
		}()
		if err := a.executeRun(context.Background()); err != nil {
			a.logger.Error("测速任务失败", "error", err)
			a.stateMu.Lock()
			now := time.Now()
			a.state.LastRunAt = &now
			a.state.LastError = err.Error()
			a.stateMu.Unlock()
		}
	}()
	return nil
}

func (a *App) Scheduler(ctx context.Context) {
	delay := time.Duration(a.Config().IntervalMinutes) * time.Minute
	if a.runOnStart {
		delay = 0
	}
	for {
		a.setNextRun(time.Now().Add(delay))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			a.setNextRun(time.Time{})
			return
		case <-timer.C:
			if err := a.RunNow(); err != nil && !errors.Is(err, ErrAlreadyRunning) {
				a.logger.Warn("无法启动计划任务", "error", err)
			}
			delay = time.Duration(a.Config().IntervalMinutes) * time.Minute
		case <-a.scheduleWake:
			timer.Stop()
			delay = time.Duration(a.Config().IntervalMinutes) * time.Minute
		}
	}
}

func (a *App) executeRun(parent context.Context) error {
	cfg := a.Config()
	ctx, cancel := context.WithTimeout(parent, a.jobTimeout)
	defer cancel()

	ipFile, ipSource, err := a.prepareIPFile(ctx, cfg)
	if err != nil {
		return err
	}
	outputPath := filepath.Join(a.dataDir, "work", fmt.Sprintf("result-%d.csv", time.Now().UnixNano()))
	defer os.Remove(outputPath)
	args := []string{
		"-f", ipFile,
		"-o", outputPath,
		"-p", "0",
		"-n", "200",
		"-t", "3",
		"-dn", strconv.Itoa(cfg.Candidates),
		"-dt", strconv.Itoa(cfg.TestDurationSeconds),
		"-warmup", strconv.Itoa(cfg.WarmupSeconds),
		"-streams", strconv.Itoa(cfg.Streams),
		"-tp", strconv.Itoa(cfg.TestPort),
		"-url", cfg.TestURL,
		"-sl", strconv.FormatFloat(cfg.MinSpeedMBs, 'f', 2, 64),
		"-tl", strconv.Itoa(cfg.MaxLatencyMS),
		"-tlr", strconv.FormatFloat(cfg.MaxLossRate, 'f', 2, 64),
	}
	var output limitedBuffer
	output.limit = 1 << 20
	cmd := exec.CommandContext(ctx, a.enginePath, args...)
	cmd.Dir = filepath.Join(a.dataDir, "work")
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=dumb")
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(output.String())
		if tail == "" {
			return fmt.Errorf("CFST 执行失败: %w", err)
		}
		return fmt.Errorf("CFST 执行失败: %w\n%s", err, tail)
	}

	results, candidates, err := parseResultFile(outputPath)
	if err != nil {
		return err
	}
	now := time.Now()
	newState := runnerState{
		Ready:      true,
		UpdatedAt:  &now,
		LastRunAt:  &now,
		IPSource:   ipSource,
		Candidates: candidates,
		Results:    results,
	}
	if err := saveJSONAtomic(a.resultPath, newState); err != nil {
		return err
	}
	a.stateMu.Lock()
	a.state = newState
	a.stateMu.Unlock()
	a.logger.Info("测速任务完成", "results", len(results), "candidates", candidates, "ip_source", ipSource)
	return nil
}

func (a *App) setNextRun(value time.Time) {
	a.nextRunMu.Lock()
	a.nextRunAt = value
	a.nextRunMu.Unlock()
}

func (a *App) notifyScheduler() {
	select {
	case a.scheduleWake <- struct{}{}:
	default:
	}
}

func (a *App) authorized(header string) bool {
	const prefix = "Bearer "
	token := strings.TrimSpace(header)
	if strings.HasPrefix(token, prefix) {
		token = strings.TrimSpace(strings.TrimPrefix(token, prefix))
	}
	if token == "" || a.adminToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.adminToken)) == 1
}

func (a *App) loadOrCreateAdminToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("ADMIN_TOKEN")); token != "" {
		return token, nil
	}
	tokenPath := filepath.Join(a.dataDir, "admin_token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token, nil
		}
	}
	randomBytes := make([]byte, 24)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(randomBytes)
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	a.logger.Warn("已生成管理员令牌，请妥善保存", "token", token, "file", tokenPath)
	return token, nil
}

type limitedBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.limit {
		remove := len(b.buf) - b.limit
		b.buf = append([]byte(nil), b.buf[remove:]...)
	}
	return n, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(bytes.TrimSpace(b.buf))
}
