package config

import (
	"bytes"
	"flag"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	cfg := load(nil)

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

func TestLoad_EnvVars(t *testing.T) {
	t.Setenv("KAIROS_LISTEN_ADDR", ":9090")
	t.Setenv("KAIROS_DATABASE_URL", "postgres://u:p@db:5432/test?sslmode=disable")
	t.Setenv("KAIROS_AMQP_URL", "amqp://user:pass@rabbit:5672/")
	t.Setenv("KAIROS_QUEUE_NAME", "custom.queue")
	t.Setenv("KAIROS_POLL_INTERVAL_MS", "1000")
	t.Setenv("KAIROS_API_KEY", "secret")

	cfg := load(nil)

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

func TestLoad_Flags(t *testing.T) {
	args := []string{
		"--listen-addr=:7070",
		"--database-url=postgres://u:p@db:5432/test?sslmode=disable",
		"--amqp-url=amqp://user:pass@rabbit:5672/",
		"--queue-name=flag.queue",
		"--poll-interval-ms=2000",
		"--api-key=flagkey",
	}

	cfg := load(args)

	if cfg.ListenAddr != ":7070" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":7070")
	}
	if cfg.DatabaseURL != "postgres://u:p@db:5432/test?sslmode=disable" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.AMQPURL != "amqp://user:pass@rabbit:5672/" {
		t.Errorf("AMQPURL = %q", cfg.AMQPURL)
	}
	if cfg.QueueName != "flag.queue" {
		t.Errorf("QueueName = %q", cfg.QueueName)
	}
	if cfg.PollInterval != 2000*time.Millisecond {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 2000*time.Millisecond)
	}
	if cfg.APIKey != "flagkey" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "flagkey")
	}
}

func TestLoad_FlagsTakePriorityOverEnvVars(t *testing.T) {
	t.Setenv("KAIROS_LISTEN_ADDR", ":9090")
	t.Setenv("KAIROS_API_KEY", "envkey")
	t.Setenv("KAIROS_POLL_INTERVAL_MS", "9999")

	cfg := load([]string{"--listen-addr=:7070", "--api-key=flagkey", "--poll-interval-ms=100"})

	if cfg.ListenAddr != ":7070" {
		t.Errorf("ListenAddr = %q, want flag value %q", cfg.ListenAddr, ":7070")
	}
	if cfg.APIKey != "flagkey" {
		t.Errorf("APIKey = %q, want flag value %q", cfg.APIKey, "flagkey")
	}
	if cfg.PollInterval != 100*time.Millisecond {
		t.Errorf("PollInterval = %v, want flag value %v", cfg.PollInterval, 100*time.Millisecond)
	}
}

func TestLoad_InvalidPollIntervalEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("KAIROS_POLL_INTERVAL_MS", "not-a-number")

	cfg := load(nil)

	if cfg.PollInterval != 5000*time.Millisecond {
		t.Errorf("PollInterval = %v, want default %v", cfg.PollInterval, 5000*time.Millisecond)
	}
}

// --- printUsage tests ---

func usageOutput(t *testing.T, fs *flag.FlagSet) string {
	t.Helper()
	var buf bytes.Buffer
	if err := printUsage(&buf, fs); err != nil {
		t.Fatalf("printUsage: %v", err)
	}
	return buf.String()
}

func TestPrintUsage_Header(t *testing.T) {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	out := usageOutput(t, fs)
	if !strings.HasPrefix(out, "Usage of kairos:\n") {
		t.Errorf("output missing header, got:\n%s", out)
	}
}

func TestPrintUsage_DoubleDashPrefix(t *testing.T) {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	fs.String("my-flag", "default", "some usage")
	out := usageOutput(t, fs)
	if !strings.Contains(out, "  --my-flag") {
		t.Errorf("expected --my-flag in output, got:\n%s", out)
	}
	if strings.Contains(out, "  -my-flag") && !strings.Contains(out, "  --my-flag") {
		t.Errorf("output uses single dash instead of double dash:\n%s", out)
	}
}

func TestPrintUsage_StringDefaultQuoted(t *testing.T) {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	fs.String("addr", ":8080", "listen address")
	out := usageOutput(t, fs)
	if !strings.Contains(out, `(default ":8080")`) {
		t.Errorf("string default should be quoted, got:\n%s", out)
	}
}

func TestPrintUsage_IntDefaultUnquoted(t *testing.T) {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	fs.Int64("poll-ms", 5000, "poll interval")
	out := usageOutput(t, fs)
	if !strings.Contains(out, "(default 5000)") {
		t.Errorf("int default should be unquoted, got:\n%s", out)
	}
	if strings.Contains(out, `(default "5000")`) {
		t.Errorf("int default should not be quoted, got:\n%s", out)
	}
}

func TestPrintUsage_EmptyStringDefaultOmitted(t *testing.T) {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	fs.String("api-key", "", "api key")
	out := usageOutput(t, fs)
	if strings.Contains(out, "(default") {
		t.Errorf("empty default should be omitted, got:\n%s", out)
	}
}

func TestPrintUsage_ZeroIntDefaultShown(t *testing.T) {
	// Unlike Go's flag.PrintDefaults, we don't suppress zero int defaults —
	// "0" is a non-empty string so the (default 0) line is printed.
	// None of our real flags default to 0, so this is acceptable.
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	fs.Int64("count", 0, "some count")
	out := usageOutput(t, fs)
	if !strings.Contains(out, "(default 0)") {
		t.Errorf("expected (default 0) for zero int, got:\n%s", out)
	}
}

func TestPrintUsage_UsageTextIncluded(t *testing.T) {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	fs.String("queue-name", "kairos.triggers", "AMQP queue name (KAIROS_QUEUE_NAME)")
	out := usageOutput(t, fs)
	if !strings.Contains(out, "AMQP queue name (KAIROS_QUEUE_NAME)") {
		t.Errorf("usage text missing from output:\n%s", out)
	}
}

func TestPrintUsage_MultipleFlagsAllPrefixed(t *testing.T) {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	fs.String("alpha", "a", "first")
	fs.String("beta", "b", "second")
	fs.Int64("gamma", 3, "third")
	out := usageOutput(t, fs)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(out, "  --"+name) {
			t.Errorf("flag %q missing --prefix in output:\n%s", name, out)
		}
	}
}

func TestPrintUsage_TypeNamesPresent(t *testing.T) {
	fs := flag.NewFlagSet("kairos", flag.ContinueOnError)
	fs.String("name", "", "a string flag")
	fs.Int64("count", 0, "an int flag")
	out := usageOutput(t, fs)
	if !strings.Contains(out, "--name string") {
		t.Errorf("expected 'string' type name, got:\n%s", out)
	}
	if !strings.Contains(out, "--count int") {
		t.Errorf("expected 'int' type name, got:\n%s", out)
	}
}
