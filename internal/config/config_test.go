package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func validEnv() map[string]string {
	return map[string]string{
		EnvAPIAddr:           "127.0.0.1:8000",
		EnvWorkerAddr:        "127.0.0.1:8001",
		EnvDatabaseURL:       "postgres://cursus:secret@127.0.0.1:5432/cursus",
		EnvTemporalAddress:   "127.0.0.1:7233",
		EnvTemporalNamespace: "default",
		EnvLogLevel:          "debug",
		EnvReadinessTimeout:  "2s",
		EnvTokenSigningKey:   "test-signing-key-for-unit-tests-only",
	}
}

func lookup(env map[string]string) func(string) string {
	return func(name string) string { return env[name] }
}

func TestLoadReadsEachProcessAddress(t *testing.T) {
	api, err := LoadAPI(lookup(validEnv()))
	if err != nil {
		t.Fatalf("LoadAPI: %v", err)
	}
	worker, err := LoadWorker(lookup(validEnv()))
	if err != nil {
		t.Fatalf("LoadWorker: %v", err)
	}

	if api.Addr != "127.0.0.1:8000" || worker.Addr != "127.0.0.1:8001" {
		t.Errorf("addresses = %q and %q", api.Addr, worker.Addr)
	}
	if api.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug", api.LogLevel)
	}
	if api.ReadinessTimeout != 2*time.Second {
		t.Errorf("ReadinessTimeout = %v, want 2s", api.ReadinessTimeout)
	}
}

func TestLoadNamesEveryMissingVariable(t *testing.T) {
	env := validEnv()
	delete(env, EnvDatabaseURL)
	delete(env, EnvTemporalAddress)

	_, err := LoadAPI(lookup(env))
	if err == nil {
		t.Fatal("LoadAPI succeeded with variables missing")
	}
	for _, name := range []string{EnvDatabaseURL, EnvTemporalAddress} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

func TestLoadDoesNotRequireTheOtherProcessAddress(t *testing.T) {
	env := validEnv()
	delete(env, EnvWorkerAddr)

	if _, err := LoadAPI(lookup(env)); err != nil {
		t.Fatalf("LoadAPI needs %s: %v", EnvWorkerAddr, err)
	}
}

func TestOnlyTheAPIRequiresTheSigningKey(t *testing.T) {
	env := validEnv()
	delete(env, EnvTokenSigningKey)

	worker, err := LoadWorker(lookup(env))
	if err != nil {
		t.Fatalf("LoadWorker needs %s: %v", EnvTokenSigningKey, err)
	}
	if worker.TokenSigningKey != nil {
		t.Error("the worker configuration carries a signing key")
	}

	_, err = LoadAPI(lookup(env))
	if err == nil || !strings.Contains(err.Error(), EnvTokenSigningKey) {
		t.Errorf("LoadAPI without the key: error = %v, want one naming %s", err, EnvTokenSigningKey)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]struct{ name, value string }{
		"unknown log level":    {EnvLogLevel, "loud"},
		"malformed timeout":    {EnvReadinessTimeout, "soon"},
		"non-positive timeout": {EnvReadinessTimeout, "0s"},
	}
	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			env := validEnv()
			env[tc.name] = tc.value

			_, err := LoadAPI(lookup(env))
			if err == nil {
				t.Fatalf("LoadAPI accepted %s=%q", tc.name, tc.value)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("error %q does not name %s", err, tc.name)
			}
		})
	}
}
