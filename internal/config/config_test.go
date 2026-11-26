package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenFileMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != ":8080" {
		t.Fatalf("listen = %q, want :8080", cfg.Server.Listen)
	}
	if cfg.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q", cfg.Timezone)
	}
	if cfg.Security.SessionTTLHours != 12 {
		t.Fatalf("session ttl = %d", cfg.Security.SessionTTLHours)
	}
}

func TestLoadYAMLOverridesDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	body := "server:\n  listen: \":9090\"\ndata_dir: /tmp/spool-test\ntimezone: UTC\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != ":9090" || cfg.DataDir != "/tmp/spool-test" || cfg.Timezone != "UTC" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("default log level lost: %q", cfg.Logging.Level)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("SPOOL_LISTEN", ":7070")
	t.Setenv("SPOOL_LOG_LEVEL", "debug")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != ":7070" || cfg.Logging.Level != "debug" {
		t.Fatalf("env override not applied: %+v", cfg)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cases := map[string]Config{
		"bad timezone":      func() Config { c := Default(); c.Timezone = "Mars/Base"; return c }(),
		"bad host key":      func() Config { c := Default(); c.Security.HostKeyPolicy = "whatever"; return c }(),
		"bad log level":     func() Config { c := Default(); c.Logging.Level = "verbose"; return c }(),
		"zero session ttl":  func() Config { c := Default(); c.Security.SessionTTLHours = 0; return c }(),
		"empty data dir":    func() Config { c := Default(); c.DataDir = ""; return c }(),
		"empty listen":      func() Config { c := Default(); c.Server.Listen = "  "; return c }(),
		"zero lockout":      func() Config { c := Default(); c.Security.LoginLockoutMinutes = 0; return c }(),
		"zero max failures": func() Config { c := Default(); c.Security.LoginMaxFailures = 0; return c }(),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if err := c.Validate(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLocation(t *testing.T) {
	c := Default()
	loc, err := c.Location()
	if err != nil {
		t.Fatalf("Location: %v", err)
	}
	if loc.String() != "Asia/Shanghai" {
		t.Fatalf("loc = %q", loc.String())
	}
}
