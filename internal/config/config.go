package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	EnvDev  = "dev"
	EnvProd = "prod"
)

type Config struct {
	AppEnv                string
	TracingEnabled        bool
	TracingEndpoint       string
	TracingSampleRatio    float64
	TracingServiceName    string
	TracingServiceVersion string
	Port                  string
	DatabaseDSN           string
	DBMaxConns            int32
	DBMinConns            int32
	DBMaxConnLife         int // seconds
	DBMaxConnIdle         int // seconds
	KafkaBrokers          []string
	KafkaGroupID          string
	JWTSecret             string
	JWTExpMinutes         int
	RefreshExpDays        int
	AllowedOrigins        []string
	RateLimit             int
}

func Load() *Config {
	return &Config{
		AppEnv:                getEnv("APP_ENV", EnvDev),
		TracingEnabled:        getEnvBool("TRACING_ENABLED", false),
		TracingEndpoint:       getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318/v1/traces"),
		TracingSampleRatio:    getEnvFloat64("OTEL_TRACES_SAMPLER_ARG", 0.1),
		TracingServiceName:    getEnv("OTEL_SERVICE_NAME", "cloudtalk"),
		TracingServiceVersion: getEnv("OTEL_SERVICE_VERSION", "dev"),
		Port:                  getEnv("PORT", "8080"),
		DatabaseDSN:           getEnv("DATABASE_DSN", "postgres://postgres:postgres@localhost:5432/cloudtalk?sslmode=disable"),
		DBMaxConns:            getEnvInt32("DB_MAX_CONNS", 20),
		DBMinConns:            getEnvInt32("DB_MIN_CONNS", 2),
		DBMaxConnLife:         getEnvInt("DB_MAX_CONN_LIFE_SECS", 3600),
		DBMaxConnIdle:         getEnvInt("DB_MAX_CONN_IDLE_SECS", 300),
		KafkaBrokers:          splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaGroupID:          getEnv("KAFKA_GROUP_ID", hostnameOrDefault("cloudtalk")),
		JWTSecret:             getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpMinutes:         getEnvInt("JWT_EXP_MINUTES", 15),
		RefreshExpDays:        getEnvInt("REFRESH_EXP_DAYS", 7),
		AllowedOrigins:        splitCSV(getEnv("ALLOWED_ORIGINS", "")),
		RateLimit:             getEnvInt("AUTH_RATE_LIMIT_RPM", 20),
	}
}

func (c *Config) Validate() error {
	var issues []string

	if c.AppEnv != EnvDev && c.AppEnv != EnvProd {
		issues = append(issues, "APP_ENV must be one of: dev, prod")
	}
	issues = append(issues, validateNumericEnvConfig()...)
	issues = append(issues, c.validateBaseConfig()...)
	issues = append(issues, c.validateOrigins()...)
	issues = append(issues, c.validateTracingConfig()...)
	if c.AppEnv == EnvProd {
		issues = append(issues, c.validateProdConfig()...)
	}

	if len(issues) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(issues, "; "))
	}
	return nil
}

func (c *Config) validateTracingConfig() []string {
	if !c.TracingEnabled {
		return nil
	}
	var issues []string
	issues = append(issues, validateRawTraceSampleRatio("OTEL_TRACES_SAMPLER_ARG")...)
	if c.TracingEndpoint == "" {
		issues = append(issues, "OTEL_EXPORTER_OTLP_ENDPOINT must not be empty when tracing is enabled")
	} else if parsed, err := url.Parse(c.TracingEndpoint); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		issues = append(issues, "OTEL_EXPORTER_OTLP_ENDPOINT must be a valid URL when tracing is enabled")
	}
	if c.TracingServiceName == "" {
		issues = append(issues, "OTEL_SERVICE_NAME must not be empty when tracing is enabled")
	}
	if c.TracingSampleRatio < 0 || c.TracingSampleRatio > 1 {
		issues = append(issues, "OTEL_TRACES_SAMPLER_ARG must be between 0 and 1")
	}
	return issues
}

func validateRawTraceSampleRatio(key string) []string {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return []string{key + " must be a valid float"}
		}
		if n < 0 || n > 1 {
			return []string{key + " must be between 0 and 1"}
		}
	}
	return nil
}

func validateNumericEnvConfig() []string {
	var issues []string
	issues = append(issues, validateRawPositiveInt("PORT", 65535)...)
	issues = append(issues, validateRawInt32("DB_MAX_CONNS", 1, 1<<30-1)...)
	issues = append(issues, validateRawInt32("DB_MIN_CONNS", 0, 1<<30-1)...)
	issues = append(issues, validateRawPositiveInt("DB_MAX_CONN_LIFE_SECS", 1<<30-1)...)
	issues = append(issues, validateRawPositiveInt("DB_MAX_CONN_IDLE_SECS", 1<<30-1)...)
	issues = append(issues, validateRawPositiveInt("JWT_EXP_MINUTES", 1<<30-1)...)
	issues = append(issues, validateRawPositiveInt("REFRESH_EXP_DAYS", 1<<30-1)...)
	issues = append(issues, validateRawPositiveInt("AUTH_RATE_LIMIT_RPM", 1<<30-1)...)
	return issues
}

func (c *Config) validateBaseConfig() []string {
	var issues []string
	if c.DatabaseDSN == "" {
		issues = append(issues, "DATABASE_DSN must not be empty")
	}
	if len(c.KafkaBrokers) == 0 {
		issues = append(issues, "KAFKA_BROKERS must not be empty")
	}
	if c.KafkaGroupID == "" {
		issues = append(issues, "KAFKA_GROUP_ID must not be empty")
	}
	if c.JWTSecret == "" {
		issues = append(issues, "JWT_SECRET must not be empty")
	}
	if c.JWTSecret == "change-me-in-production" {
		issues = append(issues, "JWT_SECRET must not use the default placeholder value")
	}
	if c.DBMinConns > c.DBMaxConns {
		issues = append(issues, "DB_MIN_CONNS must be less than or equal to DB_MAX_CONNS")
	}
	return issues
}

func (c *Config) validateOrigins() []string {
	issues := make([]string, 0, len(c.AllowedOrigins))
	for _, origin := range c.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			issues = append(issues, fmt.Sprintf("ALLOWED_ORIGINS contains invalid origin %q", origin))
		}
	}
	return issues
}

func (c *Config) validateProdConfig() []string {
	var issues []string
	if len(c.JWTSecret) < 32 {
		issues = append(issues, "JWT_SECRET must be at least 32 characters in prod")
	}
	if len(c.AllowedOrigins) == 0 {
		issues = append(issues, "ALLOWED_ORIGINS must not be empty in prod")
	}
	if looksLocalDatabaseDSN(c.DatabaseDSN) {
		issues = append(issues, "DATABASE_DSN must not point to localhost in prod")
	}
	if allLocalBrokers(c.KafkaBrokers) {
		issues = append(issues, "KAFKA_BROKERS must not be localhost-only in prod")
	}
	return issues
}

func validateRawPositiveInt(key string, maxValue int) []string {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return []string{key + " must be a valid integer"}
		}
		if n < 1 || n > maxValue {
			return []string{fmt.Sprintf("%s must be between %d and %d", key, 1, maxValue)}
		}
	}
	return nil
}

func validateRawInt32(key string, minValue, maxValue int32) []string {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return []string{key + " must be a valid integer"}
		}
		parsed := int32(n)
		if parsed < minValue || parsed > maxValue {
			return []string{fmt.Sprintf("%s must be between %d and %d", key, minValue, maxValue)}
		}
	}
	return nil
}

func looksLocalDatabaseDSN(dsn string) bool {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return isLocalHost(host)
}

func allLocalBrokers(brokers []string) bool {
	if len(brokers) == 0 {
		return false
	}
	for _, broker := range brokers {
		host, _, err := net.SplitHostPort(broker)
		if err != nil {
			host = broker
		}
		if !isLocalHost(host) {
			return false
		}
	}
	return true
}

func isLocalHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
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

func getEnvInt32(key string, fallback int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			return int32(n)
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvFloat64(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
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
