package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/flowpulse/flowpulse/internal/version"
	"github.com/flowpulse/flowpulse/pkg/api"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	configPath := flag.String("config", "/etc/flowpulse/server.yaml", "path to config")
	flag.Parse()

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(0)
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Info().
		Str("version", version.Version).
		Str("commit", version.Commit).
		Msg("starting flowpulse-server")

	cfg, err := api.LoadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv, err := api.NewServer(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create server")
	}

	if err := srv.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("server exited with error")
	}

	log.Info().Msg("server shutdown complete")
}
