package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunReadyRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	err := run([]string{"ready"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Error(t, err, "expected missing DATABASE_URL error")
}

func TestRunWorkerRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	err := run([]string{"worker"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Error(t, err, "expected missing DATABASE_URL error")
}

func TestRunHRSyncRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	err := run([]string{"hr-sync"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Error(t, err, "expected missing DATABASE_URL error")
}
