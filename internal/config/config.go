package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
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
	return load(os.Args[1:])
}

func load(args []string) Config {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)

	listenAddr := fs.String("listen-addr", getEnv("KAIROS_LISTEN_ADDR", ":8080"), "address to listen on (KAIROS_LISTEN_ADDR)")
	databaseURL := fs.String("database-url", getEnv("KAIROS_DATABASE_URL", "postgres://kairos:kairos@localhost:5432/kairos?sslmode=disable"), "postgres connection string (KAIROS_DATABASE_URL)")
	amqpURL := fs.String("amqp-url", getEnv("KAIROS_AMQP_URL", "amqp://guest:guest@localhost:5672/"), "AMQP connection string (KAIROS_AMQP_URL)")
	queueName := fs.String("queue-name", getEnv("KAIROS_QUEUE_NAME", "kairos.triggers"), "AMQP queue name (KAIROS_QUEUE_NAME)")
	pollIntervalMs := fs.Int64("poll-interval-ms", getPollIntervalMs(), "engine poll interval in milliseconds (KAIROS_POLL_INTERVAL_MS)")
	apiKey := fs.String("api-key", getEnv("KAIROS_API_KEY", ""), "API key for authentication (KAIROS_API_KEY)")

	fs.Usage = func() {
		if err := printUsage(os.Stderr, fs); err != nil {
			os.Exit(2)
		}
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	return Config{
		ListenAddr:   *listenAddr,
		DatabaseURL:  *databaseURL,
		AMQPURL:      *amqpURL,
		QueueName:    *queueName,
		PollInterval: time.Duration(*pollIntervalMs) * time.Millisecond,
		APIKey:       *apiKey,
	}
}

// printUsage builds the full usage text and writes it in a single call so
// there is exactly one error to handle.
func printUsage(w io.Writer, fs *flag.FlagSet) error {
	var b strings.Builder
	b.WriteString("Usage of kairos:\n")
	fs.VisitAll(func(f *flag.Flag) {
		typeName, usage := flag.UnquoteUsage(f)
		s := fmt.Sprintf("  --%s", f.Name)
		if typeName != "" {
			s += " " + typeName
		}
		s += "\n    \t" + usage
		if f.DefValue != "" {
			if typeName == "string" {
				s += fmt.Sprintf(" (default %q)", f.DefValue)
			} else {
				s += fmt.Sprintf(" (default %v)", f.DefValue)
			}
		}
		b.WriteString(s + "\n")
	})
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("writing usage: %w", err)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getPollIntervalMs() int64 {
	if v := os.Getenv("KAIROS_POLL_INTERVAL_MS"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			return ms
		}
	}
	return 5000
}