package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/flowpulse/flowpulse/internal/version"
	"github.com/flowpulse/flowpulse/pkg/aggregator"
	"github.com/flowpulse/flowpulse/pkg/store/clickhouse"
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
		Msg("starting flowpulse-aggregator")

	cfg, err := aggregator.LoadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	chWriter, err := clickhouse.NewWriter(cfg.ClickHouseDSN(), cfg.ClickHouse.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create clickhouse writer")
	}
	defer chWriter.Close()

	agg, err := aggregator.New(cfg, chWriter)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create aggregator")
	}

	if err := agg.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("aggregator exited with error")
	}

	log.Info().Msg("aggregator shutdown complete")
}
