package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port           string
	DatabaseDSN    string
	DBMaxConns     int32
	DBMinConns     int32
	DBMaxConnLife  int // seconds
	DBMaxConnIdle  int // seconds
	KafkaBrokers   []string
	KafkaGroupID   string
	JWTSecret      string
	JWTExpMinutes  int
	RefreshExpDays int
	AllowedOrigins []string
	RateLimit      int
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", "8080"),
		DatabaseDSN:    getEnv("DATABASE_DSN", "postgres://postgres:postgres@localhost:5432/cloudtalk?sslmode=disable"),
		DBMaxConns:     int32(getEnvInt("DB_MAX_CONNS", 20)),
		DBMinConns:     int32(getEnvInt("DB_MIN_CONNS", 2)),
		DBMaxConnLife:  getEnvInt("DB_MAX_CONN_LIFE_SECS", 3600),
		DBMaxConnIdle:  getEnvInt("DB_MAX_CONN_IDLE_SECS", 300),
		KafkaBrokers:   []string{getEnv("KAFKA_BROKERS", "localhost:9092")},
		KafkaGroupID:   getEnv("KAFKA_GROUP_ID", hostnameOrDefault("cloudtalk")),
		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpMinutes:  getEnvInt("JWT_EXP_MINUTES", 15),
		RefreshExpDays: getEnvInt("REFRESH_EXP_DAYS", 7),
		AllowedOrigins: splitCSV(getEnv("ALLOWED_ORIGINS", "")),
		RateLimit:      getEnvInt("AUTH_RATE_LIMIT_RPM", 20),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func hostnameOrDefault(def string) string {
	if h, err := os.Hostname(); err == nil {
		return "cloudtalk-" + h
	}
	return def
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
