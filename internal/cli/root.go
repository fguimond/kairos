// Package cli wires the kairos cobra command tree and viper-based
// configuration loading.
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/yourorg/kairos/internal/config"
)

const envPrefix = "KAIROS"

var (
	cfgFile string
	v       = viper.New()
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "kairos",
		Short:         "Kairos scheduler micro-service",
		Long:          "Kairos is a scheduler micro-service that lets clients define schedules over HTTP and publishes a message to a queue every time a schedule fires.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to YAML config file (default: ./kairos.yaml or $HOME/.kairos.yaml)")

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

// Execute is the main entry point invoked by cmd/kairos/main.go.
func Execute() error {
	cobra.OnInitialize(initConfig)
	if err := newRootCmd().Execute(); err != nil {
		return fmt.Errorf("kairos: %w", err)
	}
	return nil
}

func initConfig() {
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME")
		v.SetConfigName("kairos")
	}

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			fmt.Fprintf(os.Stderr, "warning: reading config file: %v\n", err)
		}
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault(config.KeyListenAddr, config.DefaultListenAddr)
	v.SetDefault(config.KeyDatabaseURL, config.DefaultDatabaseURL)
	v.SetDefault(config.KeyAMQPURL, config.DefaultAMQPURL)
	v.SetDefault(config.KeyQueueName, config.DefaultQueueName)
	v.SetDefault(config.KeyPollIntervalMs, config.DefaultPollIntervalMs)
	v.SetDefault(config.KeyAPIKey, config.DefaultAPIKey)
}
