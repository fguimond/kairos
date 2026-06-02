package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourorg/kairos/internal/api"
	"github.com/yourorg/kairos/internal/config"
	"github.com/yourorg/kairos/internal/engine"
	"github.com/yourorg/kairos/internal/queue"
	"github.com/yourorg/kairos/internal/service"
	"github.com/yourorg/kairos/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cfg := config.Load()

	db, err := store.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := store.Migrate(db); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	publisher, err := queue.NewAMQPPublisher(cfg.AMQPURL, cfg.QueueName)
	if err != nil {
		slog.Error("failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer publisher.Close()

	scheduleStore := store.NewScheduleStore(db)
	scheduleService := service.New(scheduleStore)
	triggerEngine := engine.New(scheduleService, publisher, cfg.PollInterval)
	server := api.NewServer(scheduleService, cfg.ListenAddr)

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
}
