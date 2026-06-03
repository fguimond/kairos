package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr   string
	DatabaseURL  string
	AMQPURL      string
	QueueName    string
	PollInterval time.Duration
	APIKey       string
}

func Load() Config {
	return Config{
		ListenAddr:   getEnv("LISTEN_ADDR", ":8080"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable"),
		AMQPURL:      getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		QueueName:    getEnv("QUEUE_NAME", "kairos.triggers"),
		PollInterval: getDuration("POLL_INTERVAL_MS", 5000),
		APIKey:       getEnv("API_KEY", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallbackMs int64) time.Duration {
	if v := os.Getenv(key); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return time.Duration(fallbackMs) * time.Millisecond
}
