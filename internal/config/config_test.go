package config

import (
	"os"
	"strings"
	"testing"
)

func TestConfigValidateDevAllowsLocalDefaults(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AppEnv:         EnvDev,
		Port:           "8080",
		DatabaseDSN:    "postgres://postgres:postgres@localhost:5432/cloudtalk?sslmode=disable",
		DBMaxConns:     20,
		DBMinConns:     2,
		DBMaxConnLife:  3600,
		DBMaxConnIdle:  300,
		KafkaBrokers:   []string{"localhost:9092"},
		KafkaGroupID:   "cloudtalk-dev",
		JWTSecret:      "dev-secret-is-good-enough-for-local",
		JWTExpMinutes:  15,
		RefreshExpDays: 7,
		RateLimit:      20,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected dev config valid, got %v", err)
	}
}

func TestConfigValidateProdRejectsUnsafeSettings(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AppEnv:         EnvProd,
		Port:           "8080",
		DatabaseDSN:    "postgres://postgres:postgres@localhost:5432/cloudtalk?sslmode=disable",
		DBMaxConns:     20,
		DBMinConns:     2,
		DBMaxConnLife:  3600,
		DBMaxConnIdle:  300,
		KafkaBrokers:   []string{"localhost:9092"},
		KafkaGroupID:   "cloudtalk-prod",
		JWTSecret:      "change-me-in-production",
		JWTExpMinutes:  15,
		RefreshExpDays: 7,
		RateLimit:      20,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected prod config validation to fail")
	}
	text := err.Error()
	for _, want := range []string{
		"JWT_SECRET must not use the default placeholder value",
		"ALLOWED_ORIGINS must not be empty in prod",
		"DATABASE_DSN must not point to localhost in prod",
		"KAFKA_BROKERS must not be localhost-only in prod",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected error to contain %q, got %q", want, text)
		}
	}
}

func TestConfigValidateConnectionBounds(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AppEnv:         EnvDev,
		Port:           "8080",
		DatabaseDSN:    "postgres://postgres:postgres@db:5432/cloudtalk?sslmode=disable",
		DBMaxConns:     2,
		DBMinConns:     3,
		DBMaxConnLife:  3600,
		DBMaxConnIdle:  300,
		KafkaBrokers:   []string{"kafka:9092"},
		KafkaGroupID:   "cloudtalk-dev",
		JWTSecret:      "dev-secret-is-good-enough-for-local",
		JWTExpMinutes:  15,
		RefreshExpDays: 7,
		RateLimit:      20,
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DB_MIN_CONNS must be less than or equal to DB_MAX_CONNS") {
		t.Fatalf("expected DB bounds validation error, got %v", err)
	}
}

func TestConfigValidateInvalidOrigin(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AppEnv:         EnvDev,
		Port:           "8080",
		DatabaseDSN:    "postgres://postgres:postgres@db:5432/cloudtalk?sslmode=disable",
		DBMaxConns:     20,
		DBMinConns:     2,
		DBMaxConnLife:  3600,
		DBMaxConnIdle:  300,
		KafkaBrokers:   []string{"kafka:9092"},
		KafkaGroupID:   "cloudtalk-dev",
		JWTSecret:      "dev-secret-is-good-enough-for-local",
		JWTExpMinutes:  15,
		RefreshExpDays: 7,
		AllowedOrigins: []string{"not-an-origin"},
		RateLimit:      20,
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "ALLOWED_ORIGINS contains invalid origin") {
		t.Fatalf("expected invalid origin error, got %v", err)
	}
}

//nolint:paralleltest // This test mutates process env to verify tracing config validation.
func TestConfigValidateTracingConfig(t *testing.T) {
	original, hadOriginal := os.LookupEnv("OTEL_TRACES_SAMPLER_ARG")
	if err := os.Unsetenv("OTEL_TRACES_SAMPLER_ARG"); err != nil {
		t.Fatalf("unset OTEL_TRACES_SAMPLER_ARG: %v", err)
	}
	t.Cleanup(func() {
		if hadOriginal {
			_ = os.Setenv("OTEL_TRACES_SAMPLER_ARG", original)
			return
		}
		_ = os.Unsetenv("OTEL_TRACES_SAMPLER_ARG")
	})

	base := Config{
		AppEnv:                EnvDev,
		TracingEnabled:        true,
		TracingEndpoint:       "http://localhost:4318/v1/traces",
		TracingSampleRatio:    0.25,
		TracingServiceName:    "cloudtalk",
		TracingServiceVersion: "dev",
		Port:                  "8080",
		DatabaseDSN:           "postgres://postgres:postgres@db:5432/cloudtalk?sslmode=disable",
		DBMaxConns:            20,
		DBMinConns:            2,
		DBMaxConnLife:         3600,
		DBMaxConnIdle:         300,
		KafkaBrokers:          []string{"kafka:9092"},
		KafkaGroupID:          "cloudtalk-dev",
		JWTSecret:             "dev-secret-is-good-enough-for-local",
		JWTExpMinutes:         15,
		RefreshExpDays:        7,
		RateLimit:             20,
	}

	if err := base.Validate(); err != nil {
		t.Fatalf("expected tracing config valid, got %v", err)
	}

	tests := []struct {
		name       string
		mutate     func(*Config)
		wantErrSub string
	}{
		{
			name: "invalid sample ratio",
			mutate: func(cfg *Config) {
				cfg.TracingSampleRatio = 1.5
			},
			wantErrSub: "OTEL_TRACES_SAMPLER_ARG must be between 0 and 1",
		},
		{
			name: "empty endpoint",
			mutate: func(cfg *Config) {
				cfg.TracingEndpoint = ""
			},
			wantErrSub: "OTEL_EXPORTER_OTLP_ENDPOINT must not be empty when tracing is enabled",
		},
		{
			name: "invalid endpoint",
			mutate: func(cfg *Config) {
				cfg.TracingEndpoint = "localhost:4318/v1/traces"
			},
			wantErrSub: "OTEL_EXPORTER_OTLP_ENDPOINT must be a valid URL when tracing is enabled",
		},
		{
			name: "empty service name",
			mutate: func(cfg *Config) {
				cfg.TracingServiceName = ""
			},
			wantErrSub: "OTEL_SERVICE_NAME must not be empty when tracing is enabled",
		},
	}

	//nolint:paralleltest // Subtests share env-sensitive setup from the parent test.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErrSub, err)
			}
		})
	}
}
