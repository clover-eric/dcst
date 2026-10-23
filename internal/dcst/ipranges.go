package dcst

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultCloudflareIPsURL = "https://api.cloudflare.com/client/v4/ips"

type ipAPIResponse struct {
	Success bool `json:"success"`
	Result  struct {
		IPv4 []string `json:"ipv4_cidrs"`
		IPv6 []string `json:"ipv6_cidrs"`
	} `json:"result"`
}

func (a *App) prepareIPFile(ctx context.Context, cfg Config) (string, string, error) {
	path := filepath.Join(a.dataDir, "work", "ip-ranges.txt")
	ipSourceURL := envString("CLOUDFLARE_IPS_URL", defaultCloudflareIPsURL)
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ipSourceURL, nil)
	if err == nil {
		resp, requestErr := client.Do(req)
		if requestErr == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
				if readErr == nil {
					var payload ipAPIResponse
					if json.Unmarshal(body, &payload) == nil && payload.Success {
						lines := append([]string{}, payload.Result.IPv4...)
						if cfg.UseIPv6 {
							lines = append(lines, payload.Result.IPv6...)
						}
						if len(lines) > 0 {
							if writeErr := writeLinesAtomic(path, lines); writeErr == nil {
								return path, "cloudflare-api", nil
							}
						}
					}
				}
			}
		}
	}

	bundledDir := envString("BUNDLED_IP_DIR", "/app")
	lines, fallbackErr := readBundledRanges(bundledDir, cfg.UseIPv6)
	if fallbackErr != nil {
		return "", "", fmt.Errorf("获取 Cloudflare IP 段失败: %w", errors.Join(err, fallbackErr))
	}
	if err := writeLinesAtomic(path, lines); err != nil {
		return "", "", err
	}
	return path, "bundled", nil
}

func readBundledRanges(dir string, includeIPv6 bool) ([]string, error) {
	lines, err := readLines(filepath.Join(dir, "ip.txt"))
	if err != nil {
		return nil, err
	}
	if includeIPv6 {
		v6, v6Err := readLines(filepath.Join(dir, "ipv6.txt"))
		if v6Err != nil {
			return nil, v6Err
		}
		lines = append(lines, v6...)
	}
	if len(lines) == 0 {
		return nil, errors.New("内置 IP 列表为空")
	}
	return lines, nil
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func writeLinesAtomic(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ip-ranges-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	for _, line := range lines {
		if _, err := fmt.Fprintln(tmp, line); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
