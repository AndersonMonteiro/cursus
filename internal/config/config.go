// Package config reads the configuration of the service processes from
// environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/AndersonMonteiro/cursus/internal/auth"
)

const (
	EnvAPIAddr           = "CURSUS_API_ADDR"
	EnvWorkerAddr        = "CURSUS_WORKER_ADDR"
	EnvDatabaseURL       = "CURSUS_DATABASE_URL"
	EnvTemporalAddress   = "CURSUS_TEMPORAL_ADDRESS"
	EnvTemporalNamespace = "CURSUS_TEMPORAL_NAMESPACE"
	EnvLogLevel          = "CURSUS_LOG_LEVEL"
	EnvReadinessTimeout  = "CURSUS_READINESS_TIMEOUT"
	EnvTokenSigningKey   = "CURSUS_TOKEN_SIGNING_KEY" // #nosec G101 -- name of the variable, not a credential
)

type Config struct {
	// Addr is the listen address of the process that loaded the configuration.
	Addr              string
	DatabaseURL       string
	TemporalAddress   string
	TemporalNamespace string
	LogLevel          slog.Level
	ReadinessTimeout  time.Duration
	// TokenSigningKey is set for the API only. The worker verifies no tokens
	// and is not given the key.
	TokenSigningKey []byte
}

// LoadAPI reads the configuration of the API process. getenv is os.Getenv
// outside tests.
func LoadAPI(getenv func(string) string) (Config, error) {
	return load(getenv, EnvAPIAddr, true)
}

// LoadWorker reads the configuration of the worker process.
func LoadWorker(getenv func(string) string) (Config, error) {
	return load(getenv, EnvWorkerAddr, false)
}

// load reports every missing or invalid variable at once, so a fresh
// environment is fixed in one pass instead of one failed start per variable.
func load(getenv func(string) string, addrEnv string, verifiesTokens bool) (Config, error) {
	var errs []error
	required := func(name string) string {
		value := getenv(name)
		if value == "" {
			errs = append(errs, fmt.Errorf("%s is not set", name))
		}
		return value
	}

	cfg := Config{
		Addr:              required(addrEnv),
		DatabaseURL:       required(EnvDatabaseURL),
		TemporalAddress:   required(EnvTemporalAddress),
		TemporalNamespace: required(EnvTemporalNamespace),
	}

	if raw := required(EnvLogLevel); raw != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(raw)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvLogLevel, err))
		}
	}

	if raw := required(EnvReadinessTimeout); raw != "" {
		timeout, err := time.ParseDuration(raw)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", EnvReadinessTimeout, err))
		case timeout <= 0:
			errs = append(errs, fmt.Errorf("%s must be positive, got %s", EnvReadinessTimeout, raw))
		}
		cfg.ReadinessTimeout = timeout
	}

	if verifiesTokens {
		raw := required(EnvTokenSigningKey)
		// The length is checked here as well as in package auth so that a
		// short key is reported together with every other invalid variable.
		if raw != "" && len(raw) < auth.MinKeyBytes {
			errs = append(errs, fmt.Errorf("%s must have at least %d bytes, got %d", EnvTokenSigningKey, auth.MinKeyBytes, len(raw)))
		}
		cfg.TokenSigningKey = []byte(raw)
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}
