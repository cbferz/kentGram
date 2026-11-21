package config

import (
	"log"
	"os"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
}

func Load() Config {
	cfg := Config{
		Port:        env("PORT", "8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://kentgram:kentgram_dev_only@localhost:5432/kentgram?sslmode=disable"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
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
