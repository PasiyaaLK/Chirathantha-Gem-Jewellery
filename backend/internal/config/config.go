// Package config loads runtime configuration from environment variables.
// Nothing here talks to the network or the database — it's pure, so it's
// trivial to unit test and to override in CI.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port string

	DatabaseURL       string
	DBMaxConns        int32
	DBMinConns        int32
	DBConnectTimeout  time.Duration
	DBMaxConnLifetime time.Duration

	// JWTSecret signs and verifies auth tokens (see internal/middleware).
	// Required, like DatabaseURL — an insecure baked-in default here would
	// mean every deployment that forgets to set it shares the same key.
	JWTSecret []byte
	// JWTTokenTTL controls how long a signup/login-issued token is valid.
	JWTTokenTTL time.Duration

	// AllowedOrigins configures CORS for the Next.js frontend.
	AllowedOrigins []string
}

// Load reads configuration from the environment, applying sane defaults
// for local development. DATABASE_URL and JWT_SECRET are the only
// variables that MUST be set explicitly — we refuse to guess credentials
// or secrets.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required, e.g. " +
			"postgres://user:pass@localhost:5432/gemstore?sslmode=disable")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("config: JWT_SECRET is required and must be at least 32 characters " +
			"(e.g. `openssl rand -base64 32`)")
	}

	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       dbURL,
		DBMaxConns:        getEnvInt32("DB_MAX_CONNS", 10),
		DBMinConns:        getEnvInt32("DB_MIN_CONNS", 2),
		DBConnectTimeout:  getEnvDuration("DB_CONNECT_TIMEOUT", 5*time.Second),
		DBMaxConnLifetime: getEnvDuration("DB_MAX_CONN_LIFETIME", time.Hour),
		JWTSecret:         []byte(jwtSecret),
		JWTTokenTTL:       getEnvDuration("JWT_TOKEN_TTL", 24*time.Hour),
		AllowedOrigins:    []string{getEnv("FRONTEND_ORIGIN", "http://localhost:3000")},
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt32(key string, fallback int32) int32 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return fallback
	}
	return int32(parsed)
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
