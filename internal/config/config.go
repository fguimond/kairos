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
		ListenAddr:   getEnv("KAIROS_LISTEN_ADDR", ":8080"),
		DatabaseURL:  getEnv("KAIROS_DATABASE_URL", "postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable"),
		AMQPURL:      getEnv("KAIROS_AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		QueueName:    getEnv("KAIROS_QUEUE_NAME", "kairos.triggers"),
		PollInterval: getDuration("KAIROS_POLL_INTERVAL_MS", 5000),
		APIKey:       getEnv("KAIROS_API_KEY", ""),
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
