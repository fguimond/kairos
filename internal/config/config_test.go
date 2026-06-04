package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func newViper() *viper.Viper {
	v := viper.New()
	v.SetDefault(KeyListenAddr, DefaultListenAddr)
	v.SetDefault(KeyDatabaseURL, DefaultDatabaseURL)
	v.SetDefault(KeyAMQPURL, DefaultAMQPURL)
	v.SetDefault(KeyQueueName, DefaultQueueName)
	v.SetDefault(KeyPollIntervalMs, DefaultPollIntervalMs)
	v.SetDefault(KeyAPIKey, DefaultAPIKey)
	return v
}

func TestFromViper_Defaults(t *testing.T) {
	cfg := FromViper(newViper())

	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
	if cfg.DatabaseURL != "postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.AMQPURL != "amqp://guest:guest@localhost:5672/" {
		t.Errorf("AMQPURL = %q", cfg.AMQPURL)
	}
	if cfg.QueueName != "kairos.triggers" {
		t.Errorf("QueueName = %q", cfg.QueueName)
	}
	if cfg.PollInterval != 5000*time.Millisecond {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 5000*time.Millisecond)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
}

func TestFromViper_OverridesPropagate(t *testing.T) {
	v := newViper()
	v.Set(KeyListenAddr, ":9090")
	v.Set(KeyDatabaseURL, "postgres://u:p@db:5432/test?sslmode=disable")
	v.Set(KeyAMQPURL, "amqp://user:pass@rabbit:5672/")
	v.Set(KeyQueueName, "custom.queue")
	v.Set(KeyPollIntervalMs, 1000)
	v.Set(KeyAPIKey, "secret")

	cfg := FromViper(v)

	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
	}
	if cfg.DatabaseURL != "postgres://u:p@db:5432/test?sslmode=disable" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.AMQPURL != "amqp://user:pass@rabbit:5672/" {
		t.Errorf("AMQPURL = %q", cfg.AMQPURL)
	}
	if cfg.QueueName != "custom.queue" {
		t.Errorf("QueueName = %q", cfg.QueueName)
	}
	if cfg.PollInterval != 1000*time.Millisecond {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 1000*time.Millisecond)
	}
	if cfg.APIKey != "secret" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "secret")
	}
}
