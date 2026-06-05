package main

import (
	"fmt"
	"log/slog"
	"os"

	"event-ticket-system/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("process failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	cfg := config.Load()
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "serve":
		return serve(cfg, logger)
	case "hr-sync":
		return hrSync(cfg, logger, args[1:])
	case "migrate":
		return migrate(cfg, logger)
	case "ready":
		return ready(cfg)
	case "seed":
		return seed(cfg, logger)
	case "worker":
		return worker(cfg, logger, args[1:])
	case "ops":
		return ops(cfg, logger, args[1:])
	case "admin":
		return admin(cfg, logger, args[1:])
	case "process-no-shows":
		return processNoShows(cfg, logger)
	default:
		return fmt.Errorf("unknown command")
	}
}
