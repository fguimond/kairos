package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yourorg/kairos/internal/api"
	"github.com/yourorg/kairos/internal/config"
	"github.com/yourorg/kairos/internal/engine"
	"github.com/yourorg/kairos/internal/queue"
	"github.com/yourorg/kairos/internal/store"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	cfg := config.Load()

	db, err := store.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	if err := store.Migrate(db); err != nil {
		log.Fatal().Err(err).Msg("migration failed")
	}

	publisher, err := queue.NewAMQPPublisher(cfg.AMQPURL, cfg.QueueName)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to RabbitMQ")
	}
	defer publisher.Close()

	scheduleStore := store.NewScheduleStore(db)
	triggerEngine := engine.New(scheduleStore, publisher, cfg.PollInterval)
	server := api.NewServer(scheduleStore, cfg.ListenAddr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		log.Info().Str("addr", cfg.ListenAddr).Msg("API server starting")
		if err := server.Start(); err != nil {
			log.Error().Err(err).Msg("API server stopped")
			cancel()
		}
	}()

	go func() {
		log.Info().Msg("trigger engine starting")
		triggerEngine.Run(ctx)
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down kairos")
	server.Shutdown(context.Background())
}
