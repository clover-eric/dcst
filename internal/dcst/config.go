package dcst

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func defaultConfig() Config {
	return Config{
		IntervalMinutes:     envInt("INTERVAL_MINUTES", 60),
		Candidates:          envInt("CANDIDATES", 40),
		Streams:             envInt("STREAMS", 16),
		TestDurationSeconds: envInt("TEST_DURATION_SECONDS", 5),
		WarmupSeconds:       envInt("WARMUP_SECONDS", 1),
		TestPort:            envInt("TEST_PORT", 443),
		TestURL:             envString("TEST_URL", "https://cf.xiu2.xyz/url"),
		MinSpeedMBs:         envFloat("MIN_SPEED_MB_S", 0),
		MaxLatencyMS:        envInt("MAX_LATENCY_MS", 9999),
		MaxLossRate:         envFloat("MAX_LOSS_RATE", 1),
		UseIPv6:             envBool("USE_IPV6", true),
	}
}

func (c *Config) normalize() error {
	if c.IntervalMinutes < 5 || c.IntervalMinutes > 10080 {
		return errors.New("测速间隔必须在 5 到 10080 分钟之间")
	}
	if c.Candidates < ResultLimit || c.Candidates > 500 {
		return fmt.Errorf("候选数量必须在 %d 到 500 之间", ResultLimit)
	}
	if c.Streams < 1 || c.Streams > 64 {
		return errors.New("并发流数量必须在 1 到 64 之间")
	}
	if c.TestDurationSeconds < 3 || c.TestDurationSeconds > 60 {
		return errors.New("单 IP 测试时间必须在 3 到 60 秒之间")
	}
	if c.WarmupSeconds < 0 || c.WarmupSeconds >= c.TestDurationSeconds {
		return errors.New("预热时间必须大于等于 0 且小于单 IP 测试时间")
	}
	if c.TestPort < 1 || c.TestPort > 65535 {
		return errors.New("测试端口必须在 1 到 65535 之间")
	}
	c.TestURL = strings.TrimSpace(c.TestURL)
	if c.TestURL == "" {
		return errors.New("测速地址不能为空")
	}
	req, err := http.NewRequest(http.MethodGet, c.TestURL, nil)
	if err != nil || req.URL.Host == "" || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
		return errors.New("测速地址必须是有效的 http 或 https URL")
	}
	if c.MinSpeedMBs < 0 || c.MinSpeedMBs > 100000 {
		return errors.New("最低下载速度必须在 0 到 100000 MB/s 之间")
	}
	if c.MaxLatencyMS < 1 || c.MaxLatencyMS > 9999 {
		return errors.New("最大延迟必须在 1 到 9999 ms 之间")
	}
	if c.MaxLossRate < 0 || c.MaxLossRate > 1 {
		return errors.New("最大丢包率必须在 0 到 1 之间")
	}
	return nil
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := cfg.normalize(); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func saveJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dcst-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	return value
}

func envFloat(key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64)
	if err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
