// Package config loads the open-pdf server configuration from environment
// variables. Every value is optional and has a production-safe default.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvPrefix is shared by all environment variables understood by open-pdf.
const EnvPrefix = "OPEN_PDF_"

// Config holds the runtime configuration of the server.
type Config struct {
	// Addr is the listen address, for example ":8080".
	Addr string
	// PortDrift lets the server fall back to subsequent ports when the
	// configured one is occupied (default: on).
	PortDrift bool
	// MaxUploadBytes caps the total size of one request body (uploads included).
	MaxUploadBytes int64
	// MaxFiles caps the number of file parts accepted in one request.
	MaxFiles int
	// APIToken enables bearer-token auth when non-empty.
	APIToken string
	// CORSOrigins lists allowed Access-Control-Allow-Origin values ("*" allowed).
	CORSOrigins []string
	// TempDir is the directory used for request scratch files.
	TempDir string
	// ReadTimeout bounds reading a request (including uploads).
	ReadTimeout time.Duration
	// WriteTimeout bounds writing a response.
	WriteTimeout time.Duration
	// IdleTimeout bounds keep-alive connections.
	IdleTimeout time.Duration
	// ShutdownTimeout bounds graceful shutdown after SIGTERM/SIGINT.
	ShutdownTimeout time.Duration
	// LogJSON switches structured logs to JSON (text otherwise).
	LogJSON bool
}

// Load builds a Config from the process environment and validates it.
func Load() (*Config, error) {
	cfg := &Config{
		Addr:            envString("ADDR", ":8080"),
		PortDrift:       envBool("PORT_DRIFT", true),
		MaxUploadBytes:  int64(envInt("MAX_UPLOAD_MB", 512)) * 1024 * 1024,
		MaxFiles:        envInt("MAX_FILES", 20),
		APIToken:        envString("API_TOKEN", ""),
		CORSOrigins:     envCSV("CORS_ORIGINS", []string{"*"}),
		TempDir:         envString("TMP_DIR", os.TempDir()),
		ReadTimeout:     time.Duration(envInt("READ_TIMEOUT_SEC", 300)) * time.Second,
		WriteTimeout:    time.Duration(envInt("WRITE_TIMEOUT_SEC", 300)) * time.Second,
		IdleTimeout:     time.Duration(envInt("IDLE_TIMEOUT_SEC", 120)) * time.Second,
		ShutdownTimeout: time.Duration(envInt("SHUTDOWN_TIMEOUT_SEC", 20)) * time.Second,
		LogJSON:         envBool("LOG_JSON", false),
	}

	if cfg.Addr == "" {
		return nil, errors.New("config: ADDR must not be empty")
	}
	if cfg.MaxUploadBytes <= 0 {
		return nil, errors.New("config: MAX_UPLOAD_MB must be positive")
	}
	if cfg.MaxFiles <= 0 {
		return nil, errors.New("config: MAX_FILES must be positive")
	}
	if cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 || cfg.ShutdownTimeout <= 0 {
		return nil, errors.New("config: timeouts must be positive")
	}
	return cfg, nil
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(EnvPrefix + key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(EnvPrefix + key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: ignoring invalid %s%s=%q: %v\n", EnvPrefix, key, v, err)
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(EnvPrefix + key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envCSV(key string, def []string) []string {
	v, ok := os.LookupEnv(EnvPrefix + key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
