package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	envHTTPPort = "CAT_HTTP_PORT"
	envLogLevel = "CAT_LOG_LEVEL"

	defaultHTTPPort = 8080
	defaultLogLevel = "info"
)

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("config read failed: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	if v := os.Getenv(envHTTPPort); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s=%q: %w", envHTTPPort, v, err)
		}
		cfg.Server.HTTPPort = port
	}
	if v := os.Getenv(envLogLevel); v != "" {
		cfg.Log.Level = v
	}

	if cfg.Server.HTTPPort == 0 {
		cfg.Server.HTTPPort = defaultHTTPPort
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = defaultLogLevel
	}

	return &cfg, nil
}
