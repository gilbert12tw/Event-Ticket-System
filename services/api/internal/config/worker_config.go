package config

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	WorkerKindNotification = "notification"
	WorkerKindProjection   = "projection"
	WorkerKindCompensation = "compensation"
	WorkerKindExport       = "export"
)

var supportedWorkerKinds = []string{
	WorkerKindNotification,
	WorkerKindProjection,
	WorkerKindCompensation,
	WorkerKindExport,
}

func (c Config) WithWorkerArgs(args []string) (Config, error) {
	next := c
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "":
			continue
		case arg == "--kinds":
			if i+1 >= len(args) {
				return Config{}, fmt.Errorf("worker argument %q requires a value", arg)
			}
			kinds, err := parseWorkerKinds(args[i+1])
			if err != nil {
				return Config{}, err
			}
			next.WorkerKinds = kinds
			i++
		case strings.HasPrefix(arg, "--kinds="):
			kinds, err := parseWorkerKinds(strings.TrimPrefix(arg, "--kinds="))
			if err != nil {
				return Config{}, err
			}
			next.WorkerKinds = kinds
		default:
			return Config{}, fmt.Errorf("unknown worker argument")
		}
	}
	return next, nil
}

func parseWorkerKindsEnv(loadErrors *[]string) []string {
	kinds, err := parseWorkerKinds(getEnv("WORKER_KINDS", strings.Join(supportedWorkerKinds, ",")))
	if err != nil {
		*loadErrors = append(*loadErrors, err.Error())
		return defaultWorkerKinds()
	}
	return kinds
}

func parseWorkerKinds(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("WORKER_KINDS must include at least one worker kind")
	}
	if value == "*" {
		return defaultWorkerKinds(), nil
	}
	seen := map[string]struct{}{}
	parts := strings.Split(value, ",")
	kinds := make([]string, 0, len(parts))
	for _, part := range parts {
		kind := strings.ToLower(strings.TrimSpace(part))
		if kind == "" {
			return nil, fmt.Errorf("WORKER_KINDS contains an empty worker kind")
		}
		if !isSupportedWorkerKind(kind) {
			return nil, fmt.Errorf("WORKER_KINDS contains unsupported worker kind")
		}
		if _, ok := seen[kind]; ok {
			return nil, fmt.Errorf("WORKER_KINDS contains duplicate worker kind")
		}
		seen[kind] = struct{}{}
		kinds = append(kinds, kind)
	}
	return kinds, nil
}

func parseWorkerConcurrencyEnv(loadErrors *[]string) map[string]int {
	defaults := defaultWorkerConcurrency()
	concurrency := make(map[string]int, len(defaults))
	for _, kind := range supportedWorkerKinds {
		envName := workerConcurrencyEnvName(kind)
		concurrency[kind] = parsePositiveIntEnv(envName, strconv.Itoa(defaults[kind]), loadErrors)
	}
	return concurrency
}

func validateWorkerKinds(kinds []string) error {
	if len(kinds) == 0 {
		return fmt.Errorf("WORKER_KINDS must include at least one worker kind")
	}
	seen := map[string]struct{}{}
	for _, kind := range kinds {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind == "" {
			return fmt.Errorf("WORKER_KINDS contains an empty worker kind")
		}
		if !isSupportedWorkerKind(kind) {
			return fmt.Errorf("WORKER_KINDS contains unsupported worker kind")
		}
		if _, ok := seen[kind]; ok {
			return fmt.Errorf("WORKER_KINDS contains duplicate worker kind")
		}
		seen[kind] = struct{}{}
	}
	return nil
}

func validateWorkerConcurrency(concurrency map[string]int) error {
	if concurrency == nil {
		return fmt.Errorf("WORKER_CONCURRENCY is required")
	}
	for _, kind := range supportedWorkerKinds {
		value, ok := concurrency[kind]
		if !ok || value <= 0 {
			return fmt.Errorf("%s must be positive", workerConcurrencyEnvName(kind))
		}
	}
	return nil
}

func defaultWorkerKinds() []string {
	return append([]string(nil), supportedWorkerKinds...)
}

func defaultWorkerConcurrency() map[string]int {
	return map[string]int{
		WorkerKindNotification: 4,
		WorkerKindProjection:   2,
		WorkerKindCompensation: 1,
		WorkerKindExport:       1,
	}
}

func isSupportedWorkerKind(kind string) bool {
	for _, supported := range supportedWorkerKinds {
		if kind == supported {
			return true
		}
	}
	return false
}

func workerConcurrencyEnvName(kind string) string {
	return "WORKER_CONCURRENCY_" + strings.ToUpper(kind)
}
