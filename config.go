package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Email        string
	PIN          string
	GymId        int
	GymName      string
	PollInterval time.Duration
	OTLPEndpoint string
}

func LoadConfig() (*Config, error) {
	email := os.Getenv("PUREGYM_EMAIL")
	pin := os.Getenv("PUREGYM_PIN")
	gymIDStr := os.Getenv("PUREGYM_GYM_ID")

	if email == "" || pin == "" || gymIDStr == "" {
		return nil, fmt.Errorf("PUREGYM_EMAIL, PUREGYM_PIN and PUREGYM_GYM_ID are required")
	}

	gymId, err := strconv.Atoi(gymIDStr)
	if err != nil {
		return nil, fmt.Errorf("PUREGYM_GYM_ID must be an integer: %w", err)
	}

	cfg := &Config{
		Email:        email,
		PIN:          pin,
		GymId:        gymId,
		GymName:      envOr("PUREGYM_GYM_NAME", "Gym Name Not Set"),
		PollInterval: envDurationOr("POLL_INTERVAL", 5*time.Minute),
		OTLPEndpoint: envOr("OTEL_EXPORTER_OTLP_ENDPOINT", "alloy:4317"),
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
