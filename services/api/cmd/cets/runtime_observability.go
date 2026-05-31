package main

import (
	"context"
	"log/slog"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/observability"
)

func startRuntimeObservability(cfg config.Config, logger *slog.Logger) (*observability.Runtime, error) {
	return observability.StartRuntime(context.Background(), observability.RuntimeConfig{
		ServiceName:                  cfg.OTelServiceName,
		ServiceVersion:               cfg.OTelServiceVersion,
		DeploymentEnvironment:        cfg.AppEnv,
		OTelTracesEnabled:            cfg.OTelTracesEnabled,
		OTelExporterOTLPEndpoint:     cfg.OTelEndpoint,
		PyroscopeEnabled:             cfg.PyroscopeEnabled,
		PyroscopeServerAddress:       cfg.PyroscopeAddress,
		PyroscopeApplicationName:     cfg.PyroscopeAppName,
		PyroscopeUploadRate:          15 * time.Second,
		PyroscopeMutexBlockProfiling: true,
	}, logger)
}
