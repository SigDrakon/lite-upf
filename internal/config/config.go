// Package config loads and validates the lite-upf runtime configuration.
//
// All lab-specific values (listen addresses, interface names, log level)
// come from the configuration file so that the binary stays free of
// hard-coded paths and host-specific assumptions.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultPath is the configuration file used when no path is supplied.
const DefaultPath = "config.yaml"

// Config is the root of the runtime configuration.
type Config struct {
	N4       N4       `yaml:"n4"`
	N3       N3       `yaml:"n3"`
	N6       N6       `yaml:"n6"`
	Logging  Logging  `yaml:"logging"`
	Features Features `yaml:"features"`
}

// N4 is the PFCP control-plane endpoint (SMF <-> UPF).
type N4 struct {
	Listen string `yaml:"listen"`
}

// N3 is the GTP-U access-side endpoint (RAN <-> UPF).
type N3 struct {
	Listen string `yaml:"listen"`
}

// N6 describes the data-network side used for downlink forwarding.
type N6 struct {
	Interface string `yaml:"interface"`
}

// Logging controls log verbosity.
type Logging struct {
	Level string `yaml:"level"`
}

// Features toggles optional protocol paths so a build can be brought up
// incrementally (see AGENTS.md, "Incremental Protocol Support").
type Features struct {
	PFCP bool `yaml:"pfcp"`
	GTPU bool `yaml:"gtpu"`
}

// Default returns the configuration used when no file is provided.
func Default() *Config {
	return &Config{
		N4:      N4{Listen: "0.0.0.0:8805"},
		N3:      N3{Listen: "0.0.0.0:2152"},
		N6:      N6{Interface: "eth0"},
		Logging: Logging{Level: "info"},
		Features: Features{
			PFCP: true,
			GTPU: true,
		},
	}
}

// Load reads and validates the configuration file at path.
// An empty path selects DefaultPath.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %q: %w", path, err)
	}
	return cfg, nil
}

// Validate checks that every mandatory setting is present and well formed.
// It returns an error describing every problem it found, so a broken config
// can be fixed in one pass instead of one restart at a time.
func (c *Config) Validate() error {
	var errs []error

	if c.Features.PFCP {
		if err := validateListen("n4.listen", c.N4.Listen); err != nil {
			errs = append(errs, err)
		}
	}
	if c.Features.GTPU {
		if err := validateListen("n3.listen", c.N3.Listen); err != nil {
			errs = append(errs, err)
		}
	}

	if c.N6.Interface == "" {
		errs = append(errs, errors.New("n6.interface: must not be empty"))
	} else if strings.ContainsAny(c.N6.Interface, " \t") {
		errs = append(errs, fmt.Errorf("n6.interface: %q contains whitespace", c.N6.Interface))
	}

	if err := validateLogLevel(c.Logging.Level); err != nil {
		errs = append(errs, err)
	}

	if !c.Features.PFCP && !c.Features.GTPU {
		errs = append(errs, errors.New("features: at least one of pfcp or gtpu must be enabled"))
	}

	return errors.Join(errs...)
}

// validateListen checks that addr is a usable host:port for UDP.
//
// A wildcard host is allowed: the documented default binds every interface.
func validateListen(field, addr string) error {
	if addr == "" {
		return fmt.Errorf("%s: must not be empty", field)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s: %q is not host:port: %w", field, addr, err)
	}
	if host == "" {
		return fmt.Errorf("%s: %q is missing an interface address", field, addr)
	}
	if net.ParseIP(host) == nil {
		return fmt.Errorf("%s: %q is not a valid IP address", field, host)
	}
	if port == "" {
		return fmt.Errorf("%s: %q is missing a port", field, addr)
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return fmt.Errorf("%s: %q has a non-numeric port", field, addr)
		}
	}
	return nil
}

// validateLogLevel checks the level against the levels the logger supports.
func validateLogLevel(level string) error {
	switch strings.ToLower(level) {
	case "trace", "debug", "info", "warn", "error":
		return nil
	case "":
		return errors.New("logging.level: must not be empty")
	default:
		return fmt.Errorf("logging.level: %q is not one of trace, debug, info, warn, error", level)
	}
}
