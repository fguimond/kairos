package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/francisguimond/kairos/internal/config"
)

// resolveConfig runs the run command's flag binding + viper resolution without
// actually starting the service. It mirrors the work runCmd.RunE does before
// it would call runService, and returns the materialised Config.
func resolveConfig(t *testing.T, args []string) config.Config {
	t.Helper()

	resetViper()

	root := &cobra.Command{Use: "kairos"}
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "")

	run := newRunCmd()
	// Replace RunE so we don't start the server; just init viper, bind
	// flags, and resolve.
	var got config.Config
	run.RunE = func(cmd *cobra.Command, _ []string) error {
		initConfig()
		if err := v.BindPFlags(cmd.Flags()); err != nil {
			t.Fatalf("bind flags: %v", err)
		}
		got = config.FromViper(v)
		return nil
	}
	root.AddCommand(run)

	root.SetArgs(append([]string{"run"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return got
}

// resetViper rebuilds the package-level viper instance so tests don't leak
// state into each other.
func resetViper() {
	v = viper.New()
	cfgFile = ""
}

func TestResolve_Defaults(t *testing.T) {
	cfg := resolveConfig(t, nil)

	if cfg.ListenAddr != config.DefaultListenAddr {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, config.DefaultListenAddr)
	}
	if cfg.PollInterval != time.Duration(config.DefaultPollIntervalMs)*time.Millisecond {
		t.Errorf("PollInterval = %v", cfg.PollInterval)
	}
}

func TestResolve_EnvOverridesDefault(t *testing.T) {
	t.Setenv("KAIROS_LISTEN_ADDR", ":9090")
	t.Setenv("KAIROS_API_KEY", "envkey")
	t.Setenv("KAIROS_POLL_INTERVAL_MS", "1500")

	cfg := resolveConfig(t, nil)

	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want :9090", cfg.ListenAddr)
	}
	if cfg.APIKey != "envkey" {
		t.Errorf("APIKey = %q, want envkey", cfg.APIKey)
	}
	if cfg.PollInterval != 1500*time.Millisecond {
		t.Errorf("PollInterval = %v, want 1.5s", cfg.PollInterval)
	}
}

func TestResolve_FlagOverridesEnv(t *testing.T) {
	t.Setenv("KAIROS_LISTEN_ADDR", ":9090")
	t.Setenv("KAIROS_API_KEY", "envkey")

	cfg := resolveConfig(t, []string{"--listen-addr=:7070", "--api-key=flagkey"})

	if cfg.ListenAddr != ":7070" {
		t.Errorf("ListenAddr = %q, want :7070", cfg.ListenAddr)
	}
	if cfg.APIKey != "flagkey" {
		t.Errorf("APIKey = %q, want flagkey", cfg.APIKey)
	}
}

func TestResolve_ShortFlagsSupported(t *testing.T) {
	cfg := resolveConfig(t, []string{"-l", ":6060", "-k", "shortkey", "-p", "250"})

	if cfg.ListenAddr != ":6060" {
		t.Errorf("ListenAddr = %q, want :6060", cfg.ListenAddr)
	}
	if cfg.APIKey != "shortkey" {
		t.Errorf("APIKey = %q, want shortkey", cfg.APIKey)
	}
	if cfg.PollInterval != 250*time.Millisecond {
		t.Errorf("PollInterval = %v, want 250ms", cfg.PollInterval)
	}
}

func TestResolve_YAMLConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kairos.yaml")
	yaml := `
listen-addr: ":7777"
api-key: "from-yaml"
poll-interval-ms: 1234
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	cfg := resolveConfig(t, []string{"--config", path})

	if cfg.ListenAddr != ":7777" {
		t.Errorf("ListenAddr = %q, want :7777", cfg.ListenAddr)
	}
	if cfg.APIKey != "from-yaml" {
		t.Errorf("APIKey = %q, want from-yaml", cfg.APIKey)
	}
	if cfg.PollInterval != 1234*time.Millisecond {
		t.Errorf("PollInterval = %v, want 1.234s", cfg.PollInterval)
	}
}

func TestResolve_FlagOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kairos.yaml")
	yaml := `
listen-addr: ":7777"
api-key: "from-yaml"
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	cfg := resolveConfig(t, []string{"--config", path, "--listen-addr=:1111"})

	if cfg.ListenAddr != ":1111" {
		t.Errorf("ListenAddr = %q, want :1111 (flag wins over yaml)", cfg.ListenAddr)
	}
	if cfg.APIKey != "from-yaml" {
		t.Errorf("APIKey = %q, want from-yaml", cfg.APIKey)
	}
}

func TestVersionCommand_PrintsVersion(t *testing.T) {
	root := newRootCmd()
	buf := &strings.Builder{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		t.Errorf("expected version output, got empty string")
	}
}
