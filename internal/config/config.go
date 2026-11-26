package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig describes HTTP listener and TLS settings.
type ServerConfig struct {
	Listen       string `yaml:"listen"`
	BaseURL      string `yaml:"base_url"`
	CookieSecure bool   `yaml:"cookie_secure"`
	TLS          struct {
		Cert string `yaml:"cert"`
		Key  string `yaml:"key"`
	} `yaml:"tls"`
}

// LoggingConfig controls server log verbosity.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// SecurityConfig holds authentication and SSH trust settings.
type SecurityConfig struct {
	SessionTTLHours     int    `yaml:"session_ttl_hours"`
	LoginMaxFailures    int    `yaml:"login_max_failures"`
	LoginLockoutMinutes int    `yaml:"login_lockout_minutes"`
	HostKeyPolicy       string `yaml:"host_key_policy"`
}

// ExecutionConfig tunes the execution engine.
type ExecutionConfig struct {
	MaxConcurrentSteps    int    `yaml:"max_concurrent_steps"`
	MaxConcurrentHostRuns int    `yaml:"max_concurrent_host_runs"`
	DefaultStepTimeoutSec int    `yaml:"default_step_timeout_sec"`
	StepOutputLimitMB     int    `yaml:"step_output_limit_mb"`
	RemoteBaseDir         string `yaml:"remote_base_dir"`
	RetentionDays         int    `yaml:"retention_days"`
}

// TransferConfig tunes file transfer limits.
type TransferConfig struct {
	MaxUploadMB   int `yaml:"max_upload_mb"`
	MaxDownloadMB int `yaml:"max_download_mb"`
	Concurrency   int `yaml:"concurrency"`
}

// Config is the root configuration loaded from YAML with env overrides.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	DataDir   string          `yaml:"data_dir"`
	Timezone  string          `yaml:"timezone"`
	Logging   LoggingConfig   `yaml:"logging"`
	Security  SecurityConfig  `yaml:"security"`
	Execution ExecutionConfig `yaml:"execution"`
	Transfer  TransferConfig  `yaml:"transfer"`
}

// Default returns the built-in configuration used when a field is omitted.
func Default() Config {
	c := Config{DataDir: "./data", Timezone: "Asia/Shanghai"}
	c.Server.Listen = ":8080"
	c.Logging.Level = "info"
	c.Security.SessionTTLHours = 12
	c.Security.LoginMaxFailures = 5
	c.Security.LoginLockoutMinutes = 5
	c.Security.HostKeyPolicy = "tofu"
	c.Execution.MaxConcurrentSteps = 20
	c.Execution.MaxConcurrentHostRuns = 50
	c.Execution.DefaultStepTimeoutSec = 300
	c.Execution.StepOutputLimitMB = 20
	c.Execution.RemoteBaseDir = "/tmp"
	c.Execution.RetentionDays = 90
	c.Transfer.MaxUploadMB = 512
	c.Transfer.MaxDownloadMB = 200
	c.Transfer.Concurrency = 10
	return c
}

// Load reads YAML from path (missing file falls back to defaults), applies
// environment overrides, then validates the result.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, &cfg); err != nil {
				return nil, fmt.Errorf("parse config: %w", err)
			}
		case !os.IsNotExist(err):
			return nil, fmt.Errorf("read config: %w", err)
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("SPOOL_LISTEN"); v != "" {
		c.Server.Listen = v
	}
	if v := os.Getenv("SPOOL_BASE_URL"); v != "" {
		c.Server.BaseURL = v
	}
	if v := os.Getenv("SPOOL_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("SPOOL_LOG_LEVEL"); v != "" {
		c.Logging.Level = v
	}
	if v := os.Getenv("SPOOL_TIMEZONE"); v != "" {
		c.Timezone = v
	}
}

// Validate checks required fields and enumerations.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Server.Listen) == "" {
		return fmt.Errorf("server.listen is required")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf("data_dir is required")
	}
	if _, err := c.Location(); err != nil {
		return err
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be debug|info|warn|error")
	}
	switch c.Security.HostKeyPolicy {
	case "tofu", "strict":
	default:
		return fmt.Errorf("security.host_key_policy must be tofu|strict")
	}
	if c.Security.SessionTTLHours <= 0 {
		return fmt.Errorf("security.session_ttl_hours must be > 0")
	}
	if c.Security.LoginMaxFailures <= 0 {
		return fmt.Errorf("security.login_max_failures must be > 0")
	}
	if c.Security.LoginLockoutMinutes <= 0 {
		return fmt.Errorf("security.login_lockout_minutes must be > 0")
	}
	if c.Execution.MaxConcurrentSteps <= 0 {
		return fmt.Errorf("execution.max_concurrent_steps must be > 0")
	}
	if c.Execution.MaxConcurrentHostRuns <= 0 {
		return fmt.Errorf("execution.max_concurrent_host_runs must be > 0")
	}
	if c.Execution.DefaultStepTimeoutSec <= 0 {
		return fmt.Errorf("execution.default_step_timeout_sec must be > 0")
	}
	if c.Execution.StepOutputLimitMB <= 0 {
		return fmt.Errorf("execution.step_output_limit_mb must be > 0")
	}
	if c.Execution.RemoteBaseDir == "" {
		return fmt.Errorf("execution.remote_base_dir is required")
	}
	if c.Execution.RetentionDays <= 0 {
		return fmt.Errorf("execution.retention_days must be > 0")
	}
	if c.Transfer.MaxUploadMB <= 0 {
		return fmt.Errorf("transfer.max_upload_mb must be > 0")
	}
	if c.Transfer.MaxDownloadMB <= 0 {
		return fmt.Errorf("transfer.max_download_mb must be > 0")
	}
	if c.Transfer.Concurrency <= 0 {
		return fmt.Errorf("transfer.concurrency must be > 0")
	}
	return nil
}

// Location resolves the configured IANA timezone.
func (c *Config) Location() (*time.Location, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", c.Timezone, err)
	}
	return loc, nil
}
