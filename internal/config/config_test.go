package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:   "defaults are valid",
			mutate: func(*Config) {},
		},
		{
			name:    "empty n4 listen",
			mutate:  func(c *Config) { c.N4.Listen = "" },
			wantErr: "n4.listen: must not be empty",
		},
		{
			name:    "n4 listen without port",
			mutate:  func(c *Config) { c.N4.Listen = "127.0.0.1" },
			wantErr: "is not host:port",
		},
		{
			name:    "n4 listen with a wildcard host is allowed",
			mutate:  func(c *Config) { c.N4.Listen = "0.0.0.0:8805" },
			wantErr: "",
		},
		{
			name:    "n4 listen with a hostname",
			mutate:  func(c *Config) { c.N4.Listen = "smf.example.com:8805" },
			wantErr: "is not a valid IP address",
		},
		{
			name:    "n4 listen with a non numeric port",
			mutate:  func(c *Config) { c.N4.Listen = "127.0.0.1:http" },
			wantErr: "non-numeric port",
		},
		{
			name:    "empty n3 listen",
			mutate:  func(c *Config) { c.N3.Listen = "" },
			wantErr: "n3.listen: must not be empty",
		},
		{
			name:    "empty n6 interface",
			mutate:  func(c *Config) { c.N6.Interface = "" },
			wantErr: "n6.interface: must not be empty",
		},
		{
			name:    "n6 interface with whitespace",
			mutate:  func(c *Config) { c.N6.Interface = "eth 0" },
			wantErr: "contains whitespace",
		},
		{
			name:    "empty log level",
			mutate:  func(c *Config) { c.Logging.Level = "" },
			wantErr: "logging.level: must not be empty",
		},
		{
			name:    "unknown log level",
			mutate:  func(c *Config) { c.Logging.Level = "verbose" },
			wantErr: "is not one of trace, debug, info, warn, error",
		},
		{
			name:    "all features disabled",
			mutate:  func(c *Config) { c.Features.PFCP = false; c.Features.GTPU = false },
			wantErr: "at least one of pfcp or gtpu must be enabled",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(cfg)

			err := cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	cfg := Default()
	cfg.N4.Listen = ""
	cfg.N3.Listen = ""
	cfg.N6.Interface = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
	for _, want := range []string{"n4.listen", "n3.listen", "n6.interface"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() = %v, want it to mention %q", err, want)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	body := `
n4:
  listen: "127.0.0.10:8805"
n3:
  listen: "127.0.0.11:2152"
n6:
  interface: "lo"
logging:
  level: "debug"
features:
  pfcp: true
  gtpu: false
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if cfg.N4.Listen != "127.0.0.10:8805" {
		t.Errorf("N4.Listen = %q, want %q", cfg.N4.Listen, "127.0.0.10:8805")
	}
	if cfg.N3.Listen != "127.0.0.11:2152" {
		t.Errorf("N3.Listen = %q, want %q", cfg.N3.Listen, "127.0.0.11:2152")
	}
	if cfg.N6.Interface != "lo" {
		t.Errorf("N6.Interface = %q, want %q", cfg.N6.Interface, "lo")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if !cfg.Features.PFCP {
		t.Error("Features.PFCP = false, want true")
	}
	if cfg.Features.GTPU {
		t.Error("Features.GTPU = true, want false")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err == nil {
		t.Fatal("Load() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Errorf("Load() = %v, want it to mention %q", err, "read config")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("n4: [unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Errorf("Load() = %v, want it to mention %q", err, "parse config")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("n4:\n  listen: \"not-an-address\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "invalid config") {
		t.Errorf("Load() = %v, want it to mention %q", err, "invalid config")
	}
}
