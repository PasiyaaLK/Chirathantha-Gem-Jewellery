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

	// JWTSecret signs and verifies access tokens (see internal/middleware).
	// Required, like DatabaseURL — an insecure baked-in default here would
	// mean every deployment that forgets to set it shares the same key.
	JWTSecret []byte
	// AccessTokenTTL controls how long a signup/login/refresh-issued
	// access token is valid — short by design (default 15m), since it
	// travels in a cookie sent on every request and a compromised one is
	// only useful for this long. RefreshTokenTTL controls the much
	// longer-lived, revocable, DB-backed refresh token used to silently
	// mint new access tokens without forcing a re-login — see
	// AuthService.RefreshAccessToken and migrations/0002_refresh_tokens.sql.
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	// AllowedOrigins configures CORS for the Next.js frontend.
	AllowedOrigins []string

	// --- Notification providers (all optional) ---------------------------
	// Any of these left unset means that channel falls back to
	// LogNotifier — see buildNotifier in cmd/api/main.go. Unlike
	// DatabaseURL/JWTSecret, the app is fully usable without these; it
	// just won't actually deliver email/SMS.
	SendGridAPIKey   string
	EmailFromAddress string
	EmailFromName    string

	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFromNumber string
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
		AccessTokenTTL:    getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:   getEnvDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
		AllowedOrigins:    []string{getEnv("FRONTEND_ORIGIN", "http://localhost:3000")},

		SendGridAPIKey:   os.Getenv("SENDGRID_API_KEY"),
		EmailFromAddress: getEnv("EMAIL_FROM_ADDRESS", "orders@example.com"),
		EmailFromName:    getEnv("EMAIL_FROM_NAME", "Gem & Jewelry Store"),

		TwilioAccountSID: os.Getenv("TWILIO_ACCOUNT_SID"),
		TwilioAuthToken:  os.Getenv("TWILIO_AUTH_TOKEN"),
		TwilioFromNumber: os.Getenv("TWILIO_PHONE_NUMBER"),
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
