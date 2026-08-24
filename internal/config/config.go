package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Addr               string
	DataDir            string
	DatabasePath       string
	UploadDir          string
	SessionSecret      []byte
	MaxUploadBytes     int64
	AppOrigin          string
	GoogleClientID     string
	GoogleClientSecret string
	AllowedOrigins     map[string]struct{}
	SMTPHost           string
	SMTPPort           string
	SMTPUsername       string
	SMTPPassword       string
	SMTPFrom           string
}

func Load() (Config, error) {
	dataDir := value("DATA_DIR", "data")
	maxUploadMB, err := strconv.ParseInt(value("MAX_UPLOAD_MB", "1024"), 10, 64)
	if err != nil || maxUploadMB < 1 {
		return Config{}, errors.New("MAX_UPLOAD_MB must be a positive integer")
	}

	secret := os.Getenv("APP_SECRET")
	if len(secret) < 32 {
		return Config{}, errors.New("APP_SECRET must contain at least 32 characters")
	}

	return Config{
		Addr:               value("ADDR", ":8080"),
		DataDir:            dataDir,
		DatabasePath:       filepath.Join(dataDir, "cineroom.db"),
		UploadDir:          filepath.Join(dataDir, "uploads"),
		SessionSecret:      []byte(secret),
		MaxUploadBytes:     maxUploadMB * 1024 * 1024,
		AppOrigin:          value("APP_ORIGIN", "http://localhost:8080"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		AllowedOrigins:     map[string]struct{}{value("APP_ORIGIN", "http://localhost:8080"): {}},
		SMTPHost:           os.Getenv("SMTP_HOST"),
		SMTPPort:           value("SMTP_PORT", "587"),
		SMTPUsername:       os.Getenv("SMTP_USERNAME"),
		SMTPPassword:       os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:           value("SMTP_FROM", "noreply@localhost"),
	}, nil
}

func value(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
