package dcst

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUpdateUnavailable = errors.New("当前部署未启用一键更新代理")

func (a *App) RequestUpdate() error {
	if a.updateEndpoint == "" {
		return ErrUpdateUnavailable
	}
	endpoint, err := url.Parse(a.updateEndpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return errors.New("更新代理地址无效")
	}
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return err
	}
	if a.updateToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.updateToken)
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接更新代理失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("更新代理返回错误: %s", message)
	}
	return nil
}
