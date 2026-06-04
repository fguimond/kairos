package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	ListenAddr   string
	DatabaseURL  string
	AMQPURL      string
	QueueName    string
	PollInterval time.Duration
	APIKey       string
}

// Keys are the viper config keys for each setting. They double as the
// canonical flag names (kebab-case) and, via viper's KAIROS_ env prefix
// and "-"→"_" replacer, the environment variable names.
const (
	KeyListenAddr     = "listen-addr"
	KeyDatabaseURL    = "database-url"
	KeyAMQPURL        = "amqp-url"
	KeyQueueName      = "queue-name"
	KeyPollIntervalMs = "poll-interval-ms"
	KeyAPIKey         = "api-key"
)

// Defaults are the fallback values used when a setting is not provided via
// flag, env, or config file.
const (
	DefaultListenAddr     = ":8080"
	DefaultDatabaseURL    = "postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable"
	DefaultAMQPURL        = "amqp://guest:guest@localhost:5672/"
	DefaultQueueName      = "kairos.triggers"
	DefaultPollIntervalMs = 5000
	DefaultAPIKey         = ""
)

// FromViper materialises a Config from the given viper instance. Callers are
// expected to have already bound flags, set the env prefix, and read any
// config file.
func FromViper(v *viper.Viper) Config {
	return Config{
		ListenAddr:   v.GetString(KeyListenAddr),
		DatabaseURL:  v.GetString(KeyDatabaseURL),
		AMQPURL:      v.GetString(KeyAMQPURL),
		QueueName:    v.GetString(KeyQueueName),
		PollInterval: time.Duration(v.GetInt64(KeyPollIntervalMs)) * time.Millisecond,
		APIKey:       v.GetString(KeyAPIKey),
	}
}
