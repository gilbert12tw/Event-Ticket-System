package config

import (
	"errors"
	"strings"
)

func (c Config) validateRuntimeObservability() error {
	if c.OTelTracesEnabled {
		if strings.TrimSpace(c.OTelEndpoint) == "" {
			return errors.New("OTEL_EXPORTER_OTLP_ENDPOINT is required when OTEL_TRACES_ENABLED=true")
		}
		if strings.TrimSpace(c.OTelServiceName) == "" {
			return errors.New("OTEL_SERVICE_NAME is required when OTEL_TRACES_ENABLED=true")
		}
	}
	if c.PyroscopeEnabled {
		if strings.TrimSpace(c.PyroscopeAddress) == "" {
			return errors.New("PYROSCOPE_SERVER_ADDRESS is required when PYROSCOPE_ENABLED=true")
		}
		if strings.TrimSpace(c.PyroscopeAppName) == "" {
			return errors.New("PYROSCOPE_APPLICATION_NAME is required when PYROSCOPE_ENABLED=true")
		}
	}
	return nil
}
