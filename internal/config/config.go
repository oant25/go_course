package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
	HTTP            HTTP
	DB              DB
}

type HTTP struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type DB struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	QueryTimeout    time.Duration
}

func Load() (Config, error) {
	var r reader

	cfg := Config{
		LogLevel:        r.logLevel("LOG_LEVEL"),
		ShutdownTimeout: r.duration("SHUTDOWN_TIMEOUT"),
		HTTP: HTTP{
			Addr:              r.string("HTTP_ADDR"),
			ReadTimeout:       r.duration("HTTP_READ_TIMEOUT"),
			ReadHeaderTimeout: r.duration("HTTP_READ_HEADER_TIMEOUT"),
			WriteTimeout:      r.duration("HTTP_WRITE_TIMEOUT"),
			IdleTimeout:       r.duration("HTTP_IDLE_TIMEOUT"),
		},
		DB: DB{
			URL:             r.string("DATABASE_URL"),
			MaxConns:        r.int32("DATABASE_MAX_CONNS", 1),
			MinConns:        r.int32("DATABASE_MIN_CONNS", 0),
			MaxConnLifetime: r.duration("DATABASE_MAX_CONN_LIFETIME"),
			ConnectTimeout:  r.duration("DATABASE_CONNECT_TIMEOUT"),
			QueryTimeout:    r.duration("DATABASE_QUERY_TIMEOUT"),
		},
	}

	if cfg.DB.MinConns > cfg.DB.MaxConns {
		r.fail("DATABASE_MIN_CONNS", fmt.Errorf("must not exceed DATABASE_MAX_CONNS (%d)", cfg.DB.MaxConns))
	}

	if len(r.errs) > 0 {
		return Config{}, fmt.Errorf("invalid config: %w", errors.Join(r.errs...))
	}
	return cfg, nil
}

type reader struct {
	errs []error
}

func (r *reader) fail(name string, err error) {
	r.errs = append(r.errs, fmt.Errorf("%s: %w", name, err))
}

func (r *reader) string(name string) string {
	v, ok := os.LookupEnv(name)
	v = strings.TrimSpace(v)
	if !ok || v == "" {
		r.fail(name, errors.New("is required"))
		return ""
	}
	return v
}

func (r *reader) duration(name string) time.Duration {
	raw := r.string(name)
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		r.fail(name, fmt.Errorf("invalid duration %q", raw))
		return 0
	}
	if d <= 0 {
		r.fail(name, fmt.Errorf("must be positive, got %s", d))
		return 0
	}
	return d
}

func (r *reader) int32(name string, minValue int32) int32 {
	raw := r.string(name)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		r.fail(name, fmt.Errorf("invalid integer %q", raw))
		return 0
	}
	if int32(n) < minValue {
		r.fail(name, fmt.Errorf("must be >= %d, got %d", minValue, n))
		return 0
	}
	return int32(n)
}

func (r *reader) logLevel(name string) slog.Level {
	raw := r.string(name)
	if raw == "" {
		return slog.LevelInfo
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(raw)); err != nil {
		r.fail(name, fmt.Errorf("unknown level %q (want debug, info, warn, error)", raw))
		return slog.LevelInfo
	}
	return lvl
}
