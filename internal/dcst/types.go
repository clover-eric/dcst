package dcst

import "time"

const (
	ResultLimit  = 20
	ServiceName  = "dcst"
	BuildVersion = "1.2.0"
)

type Config struct {
	IntervalMinutes     int     `json:"interval_minutes"`
	Candidates          int     `json:"candidates"`
	Streams             int     `json:"streams"`
	TestDurationSeconds int     `json:"test_duration_seconds"`
	WarmupSeconds       int     `json:"warmup_seconds"`
	TestPort            int     `json:"test_port"`
	TestURL             string  `json:"test_url"`
	MinSpeedMBs         float64 `json:"min_speed_mb_s"`
	MaxLatencyMS        int     `json:"max_latency_ms"`
	MaxLossRate         float64 `json:"max_loss_rate"`
	UseIPv6             bool    `json:"use_ipv6"`
}

type IPResult struct {
	IP          string  `json:"ip"`
	Sent        int     `json:"sent"`
	Received    int     `json:"received"`
	LossRate    float64 `json:"loss_rate"`
	LatencyMS   float64 `json:"latency_ms"`
	DownloadMBs float64 `json:"download_mb_s"`
	Region      string  `json:"region"`
}

type Snapshot struct {
	Ready         bool       `json:"ready"`
	Running       bool       `json:"running"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	IPSource      string     `json:"ip_source,omitempty"`
	Count         int        `json:"count"`
	Candidates    int        `json:"candidates"`
	Results       []IPResult `json:"results"`
	AdminAuth     bool       `json:"admin_auth"`
	UpdateEnabled bool       `json:"update_enabled"`
	Service       string     `json:"service"`
	Version       string     `json:"version"`
	IntervalMin   int        `json:"interval_minutes"`
}

type AuthStatus struct {
	Initialized   bool   `json:"initialized"`
	Authenticated bool   `json:"authenticated"`
	PasswordRule  string `json:"password_rule"`
}

type runnerState struct {
	Ready      bool       `json:"ready"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	IPSource   string     `json:"ip_source,omitempty"`
	Candidates int        `json:"candidates"`
	Results    []IPResult `json:"results"`
}
