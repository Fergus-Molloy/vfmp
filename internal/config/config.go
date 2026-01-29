package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	ConfigPath = flag.String("config", "", "optional config file")
	logPath    = flag.String("log-path", "", "optional path to log to")
)

// Config holds all configuration for the vfmp service
type Config struct {
	TCPAddr         string        `yaml:"tcp_addr"`
	HTTPAddr        string        `yaml:"http_addr"`
	PprofAddr       string        `yaml:"pprof_addr"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	LogLevel        string        `yaml:"log_level"`
	LogPath         string        `yaml:"log_path"`
}

func (c *Config) applyFlagOverrides() {
	if *logPath != "" {
		c.LogPath = *logPath
	}
}

// Load reads configuration from file and environment variables.
// Configuration priority (highest to lowest):
// 1. Environment variables
// 2. Config file
// 3. Defaults
func Load() (*Config, error) {
	cfg := &Config{
		TCPAddr:         ":9090",
		HTTPAddr:        ":8080",
		PprofAddr:       ":5050",
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		LogLevel:        "debug",
		LogPath:         "",
	}

	configureFlags()

	if *ConfigPath != "" {
		if err := cfg.loadFromFile(*ConfigPath); err != nil {
			return cfg, fmt.Errorf("loading config file: %w", err)
		}
	}

	cfg.applyEnvOverrides()
	cfg.applyFlagOverrides()

	return cfg, nil
}

func configureFlags() {
	flag.Parse()
}

func (c *Config) loadFromFile(path string) error {
	slog.Info("attempting to load config from path", "path", path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parsing yaml: %w", err)
	}

	return nil
}

func (c *Config) applyEnvOverrides() {
	if val := os.Getenv("TCP_ADDR"); val != "" {
		c.TCPAddr = val
	}
	if val := os.Getenv("HTTP_ADDR"); val != "" {
		c.HTTPAddr = val
	}
	if val := os.Getenv("PPROF_ADDR"); val != "" {
		c.PprofAddr = val
	}
	if val := os.Getenv("LOG_LEVEL"); val != "" {
		c.LogLevel = val
	}
	if val := os.Getenv("LOG_PATH"); val != "" {
		c.LogPath = val
	}
	if val := os.Getenv("READ_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			c.ReadTimeout = d
		}
	}
	if val := os.Getenv("WRITE_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			c.WriteTimeout = d
		}
	}
	if val := os.Getenv("SHUTDOWN_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			c.ShutdownTimeout = d
		}
	}
}
