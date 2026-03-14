package config

import (
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
