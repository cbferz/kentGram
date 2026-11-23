package config

import (
	"log"
	"os"
	"time"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	AvatarDir string
	PublicBaseURL string
	// AccessTTL is deliberately short: access tokens are stateless JWTs that
	// are additionally checked against the session table on every request.
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

func Load() Config {
	cfg := Config{
		Port:        env("PORT", "8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://kentgram:kentgram_dev_only@localhost:5432/kentgram?sslmode=disable"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		AvatarDir: env("AVATAR_DIR", "./data/avatars"),
		PublicBaseURL: env("PUBLIC_BASE_URL", "https://api.kentgram.tendercheck.pro"),
		AccessTTL:   envDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL:  envDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
	}
	if len(cfg.JWTSecret) < 32 {
		log.Fatal("JWT_SECRET must be set and contain at least 32 characters")
	}
	return cfg
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		log.Fatalf("%s must be a positive duration such as 15m or 720h", key)
	}
	return d
}
