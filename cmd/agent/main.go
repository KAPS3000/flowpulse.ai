package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/flowpulse/flowpulse/internal/version"
	"github.com/flowpulse/flowpulse/pkg/agent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	configPath := flag.String("config", "/etc/flowpulse/agent.yaml", "path to agent config")
	flag.Parse()

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(0)
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Info().
		Str("version", version.Version).
		Str("commit", version.Commit).
		Msg("starting flowpulse-agent")

	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	a, err := agent.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create agent")
	}

	if err := a.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("agent exited with error")
	}

	log.Info().Msg("agent shutdown complete")
}
