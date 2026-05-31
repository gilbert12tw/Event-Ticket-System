package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ops(cfg config.Config, logger *slog.Logger, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("ops command is required")
	}
	switch strings.TrimSpace(args[0]) {
	case "replay":
		return opsReplay(cfg, logger, args[1:])
	case "outbox-stats":
		return opsOutboxStats(cfg, logger, args[1:])
	default:
		return fmt.Errorf("unknown ops command")
	}
}

func opsOutboxStats(cfg config.Config, logger *slog.Logger, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown ops outbox-stats argument")
	}
	return withDatabase(cfg, cfg.ValidateDatabase, func(ctx context.Context, pool *pgxpool.Pool) error {
		service := newTicketingService(pool, cfg, logger)
		status, err := service.OutboxQueueStatus(ctx, ticketing.Actor{ID: "ops-outbox-stats", Role: ticketing.RoleSystemAdmin})
		if err != nil {
			return err
		}
		for _, row := range status.Queues {
			logger.Info("ops outbox stats",
				"worker_kind", row.Name,
				"pending", row.Pending,
				"in_flight", row.InFlight,
				"dead_letter", row.DeadLetter,
				"p95_age_seconds", row.P95AgeSeconds,
				"last_processed_at", outboxStatsLastProcessed(row),
			)
		}
		return nil
	})
}

func outboxStatsLastProcessed(row ticketing.OutboxQueueStatusRow) string {
	if row.LastProcessedAt == nil {
		return ""
	}
	return row.LastProcessedAt.UTC().Format(time.RFC3339)
}

func opsReplay(cfg config.Config, logger *slog.Logger, args []string) error {
	req, actor, err := parseOpsReplayArgs(args)
	if err != nil {
		return err
	}
	return withDatabase(cfg, cfg.ValidateDatabase, func(ctx context.Context, pool *pgxpool.Pool) error {
		service := newTicketingService(pool, cfg, logger)
		result, err := service.ReplayOutbox(ctx, actor, req)
		if err != nil {
			return err
		}
		enqueued := 0
		if result.EnqueuedCount != nil {
			enqueued = *result.EnqueuedCount
		}
		logger.Info("ops replay complete",
			"kind", result.Kind,
			"from", result.From.Format(time.RFC3339),
			"to", result.To.Format(time.RFC3339),
			"dry_run", result.DryRun,
			"affected_count", result.AffectedCount,
			"enqueued_count", enqueued,
			"audit_id", result.AuditID,
		)
		return nil
	})
}

func parseOpsReplayArgs(args []string) (ticketing.ReplayOutboxRequest, ticketing.Actor, error) {
	req := ticketing.ReplayOutboxRequest{DryRun: true}
	actor := ticketing.Actor{ID: "ops-replay", Role: ticketing.RoleSystemAdmin}
	fromRaw := ""
	toRaw := ""
	fs := flag.NewFlagSet("ops replay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var(replayDryRunFlag{dryRun: &req.DryRun, value: false}, "apply", "")
	fs.Var(replayDryRunFlag{dryRun: &req.DryRun, value: true}, "dry-run", "")
	fs.StringVar(&req.Kind, "kind", "", "")
	fs.StringVar(&fromRaw, "from", "", "")
	fs.StringVar(&toRaw, "to", "", "")
	fs.Var((*replayEventTypeFlags)(&req.EventTypes), "event-type", "")
	fs.Var(replayEventTypesFlag{values: &req.EventTypes}, "event-types", "")
	fs.StringVar(&actor.ID, "actor-id", actor.ID, "")

	if err := fs.Parse(args); err != nil {
		return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("unknown ops replay argument")
	}
	if fs.NArg() > 0 {
		return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("unknown ops replay argument")
	}
	if err := parseReplayTimeFlag(fromRaw, &req.From, "from"); err != nil {
		return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
	}
	if err := parseReplayTimeFlag(toRaw, &req.To, "to"); err != nil {
		return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
	}
	actor.ID = strings.TrimSpace(actor.ID)
	if actor.ID == "" {
		return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("ops replay actor id is required")
	}
	req, err := ticketing.ValidateReplayOutboxRequest(req)
	if err != nil {
		return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
	}
	return req, actor, nil
}

type replayDryRunFlag struct {
	dryRun *bool
	value  bool
}

func (f replayDryRunFlag) String() string {
	if f.dryRun == nil {
		return "true"
	}
	return fmt.Sprint(*f.dryRun)
}

func (f replayDryRunFlag) Set(string) error {
	*f.dryRun = f.value
	return nil
}

func (f replayDryRunFlag) IsBoolFlag() bool {
	return true
}

type replayEventTypeFlags []string

func (f *replayEventTypeFlags) String() string {
	return strings.Join(*f, ",")
}

func (f *replayEventTypeFlags) Set(value string) error {
	*f = append(*f, strings.TrimSpace(value))
	return nil
}

type replayEventTypesFlag struct {
	values *[]string
}

func (f replayEventTypesFlag) String() string {
	return strings.Join(*f.values, ",")
}

func (f replayEventTypesFlag) Set(value string) error {
	*f.values = append(*f.values, splitReplayEventTypes(value)...)
	return nil
}

func parseReplayTimeFlag(raw string, target *time.Time, name string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fmt.Errorf("invalid replay %s time", name)
	}
	*target = parsed
	return nil
}

func splitReplayEventTypes(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, strings.TrimSpace(part))
	}
	return values
}
