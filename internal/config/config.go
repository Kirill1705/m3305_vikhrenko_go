package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr              string
	HTTPReadTimeout       time.Duration
	HTTPReadHeaderTimeout time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
	HTTPMaxBodyBytes      int64
	LogLevel              slog.Level
	ShutdownTimeout       time.Duration

	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
	IdempotencyTTL          time.Duration
}

func Load() (Config, error) {
	var cfg Config
	var err error

	cfg.HTTPAddr, err = required("HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return Config{}, fmt.Errorf("HTTP_ADDR must have host:port format: %w", err)
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return Config{}, fmt.Errorf("HTTP_ADDR port must be an integer between 1 and 65535")
	}

	cfg.DatabaseURL, err = required("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}

	logLevel, err := required("LOG_LEVEL")
	if err != nil {
		return Config{}, err
	}
	switch strings.ToLower(logLevel) {
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "info":
		cfg.LogLevel = slog.LevelInfo
	case "warn":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL must be debug, info, warn or error")
	}

	for _, setting := range []struct {
		name  string
		value *time.Duration
	}{
		{"HTTP_READ_TIMEOUT", &cfg.HTTPReadTimeout},
		{"HTTP_READ_HEADER_TIMEOUT", &cfg.HTTPReadHeaderTimeout},
		{"HTTP_WRITE_TIMEOUT", &cfg.HTTPWriteTimeout},
		{"HTTP_IDLE_TIMEOUT", &cfg.HTTPIdleTimeout},
		{"SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
		{"DATABASE_MAX_CONN_LIFETIME", &cfg.DatabaseMaxConnLifetime},
		{"DATABASE_CONNECT_TIMEOUT", &cfg.DatabaseConnectTimeout},
		{"DATABASE_QUERY_TIMEOUT", &cfg.DatabaseQueryTimeout},
		{"IDEMPOTENCY_TTL", &cfg.IdempotencyTTL},
	} {
		*setting.value, err = positiveDuration(setting.name)
		if err != nil {
			return Config{}, err
		}
	}

	maxConns, err := integer("DATABASE_MAX_CONNS", 32, 1)
	if err != nil {
		return Config{}, err
	}
	minConns, err := integer("DATABASE_MIN_CONNS", 32, 0)
	if err != nil {
		return Config{}, err
	}
	if minConns > maxConns {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS must not exceed DATABASE_MAX_CONNS")
	}
	cfg.DatabaseMaxConns = int32(maxConns)
	cfg.DatabaseMinConns = int32(minConns)
	cfg.HTTPMaxBodyBytes, err = integer("HTTP_MAX_BODY_BYTES", 64, 1)
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func required(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func positiveDuration(name string) (time.Duration, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return duration, nil
}

func integer(name string, bits int, minimum int64) (int64, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}
	number, err := strconv.ParseInt(value, 10, bits)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if number < minimum {
		return 0, fmt.Errorf("%s must be at least %d", name, minimum)
	}
	return number, nil
}
