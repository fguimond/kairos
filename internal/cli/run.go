package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/yourorg/kairos/internal/api"
	"github.com/yourorg/kairos/internal/config"
	"github.com/yourorg/kairos/internal/engine"
	"github.com/yourorg/kairos/internal/queue"
	"github.com/yourorg/kairos/internal/service"
	"github.com/yourorg/kairos/internal/store"
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the kairos scheduler service",
		Long: "Start the HTTP API server and the background trigger engine.\n\n" +
			"Configuration is read from (in order of precedence): flags, " +
			"KAIROS_* environment variables, YAML config file, built-in defaults.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := v.BindPFlags(cmd.Flags()); err != nil {
				return fmt.Errorf("binding flags: %w", err)
			}
			return runService(config.FromViper(v))
		},
	}

	f := cmd.Flags()
	f.StringP(config.KeyListenAddr, "l", config.DefaultListenAddr, "address to listen on (KAIROS_LISTEN_ADDR)")
	f.StringP(config.KeyDatabaseURL, "d", config.DefaultDatabaseURL, "postgres connection string (KAIROS_DATABASE_URL)")
	f.StringP(config.KeyAMQPURL, "a", config.DefaultAMQPURL, "AMQP connection string (KAIROS_AMQP_URL)")
	f.StringP(config.KeyQueueName, "q", config.DefaultQueueName, "AMQP queue name (KAIROS_QUEUE_NAME)")
	f.Int64P(config.KeyPollIntervalMs, "p", config.DefaultPollIntervalMs, "engine poll interval in milliseconds (KAIROS_POLL_INTERVAL_MS)")
	f.StringP(config.KeyAPIKey, "k", config.DefaultAPIKey, "API key for authentication (KAIROS_API_KEY)")

	return cmd
}

func runService(cfg config.Config) error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	db, err := store.Connect(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	if err := store.Migrate(db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	publisher, err := queue.NewAMQPPublisher(cfg.AMQPURL, cfg.QueueName)
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	defer publisher.Close()

	scheduleStore := store.NewScheduleStore(db)
	scheduleService := service.New(scheduleStore)
	triggerEngine := engine.New(scheduleService, publisher, cfg.PollInterval)
	server := api.NewServer(scheduleService, cfg.ListenAddr, cfg.APIKey)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		slog.Info("API server starting", "addr", cfg.ListenAddr)
		if err := server.Start(); err != nil {
			slog.Error("API server stopped", "error", err)
			cancel()
		}
	}()

	go func() {
		slog.Info("trigger engine starting")
		triggerEngine.Run(ctx)
	}()

	<-ctx.Done()
	slog.Info("shutting down kairos")
	if err := server.Shutdown(context.Background()); err != nil {
		slog.Error("API server shutdown", "error", err)
	}
	return nil
}
