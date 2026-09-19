package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port         string
	DatabaseURL  string
	JWTSecret    string
	RedisURL     string
	Env          string
	CookieSecure bool
	FrontendURL  string
}

func Load() Config {
	port := env("PORT", "8080")
	return Config{
		Port:         port,
		DatabaseURL:  env("DATABASE_URL", "postgresql://neondb_owner:npg_i82tuAwCBhZN@ep-tiny-heart-a5d9g91o-pooler.us-east-2.aws.neon.tech/neondb?sslmode=require&channel_binding=require"),
		JWTSecret:    env("JWT_SECRET", "dev-jwt-secret-change-in-production-32chars!"),
		RedisURL:     env("REDIS_URL", ""),
		Env:          env("ENV", "development"),
		CookieSecure: env("ENV", "development") == "production",
		FrontendURL:  env("FRONTEND_URL", "http://localhost:3000"),
	}
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func MustInt(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
