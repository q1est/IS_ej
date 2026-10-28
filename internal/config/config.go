package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
	JWTSecret       []byte
	AllowedOrigins  []string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	DBMaxOpenConns  int
	DBMaxIdleConns  int
}

func Load() (Config, error) {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return Config{}, errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	port := env("PORT", "8080")
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, errors.New("PORT must be an integer from 1 to 65535")
	}
	accessMinutes, err := positiveInt("ACCESS_TOKEN_MINUTES", 15)
	if err != nil {
		return Config{}, err
	}
	refreshDays, err := positiveInt("REFRESH_TOKEN_DAYS", 14)
	if err != nil {
		return Config{}, err
	}
	maxOpen, err := positiveInt("DB_MAX_OPEN_CONNS", 8)
	if err != nil {
		return Config{}, err
	}
	maxIdle, err := positiveInt("DB_MAX_IDLE_CONNS", 2)
	if err != nil {
		return Config{}, err
	}
	if maxIdle > maxOpen {
		return Config{}, errors.New("DB_MAX_IDLE_CONNS cannot exceed DB_MAX_OPEN_CONNS")
	}

	var origins []string
	for _, origin := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if value := strings.TrimSpace(origin); value != "" {
			parsed, err := url.Parse(value)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
				return Config{}, fmt.Errorf("ALLOWED_ORIGINS must contain only HTTP(S) origins: %q", value)
			}
			origins = append(origins, value)
		}
	}
	if len(origins) == 0 {
		return Config{}, errors.New("ALLOWED_ORIGINS must contain at least one HTTP(S) origin")
	}
	return Config{
		Port:            port,
		DatabaseURL:     dsn,
		JWTSecret:       []byte(secret),
		AllowedOrigins:  origins,
		AccessTokenTTL:  time.Duration(accessMinutes) * time.Minute,
		RefreshTokenTTL: time.Duration(refreshDays) * 24 * time.Hour,
		DBMaxOpenConns:  maxOpen,
		DBMaxIdleConns:  maxIdle,
	}, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func positiveInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}
