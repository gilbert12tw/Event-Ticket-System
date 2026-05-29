package main

import (
	"context"
	"fmt"
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
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "":
			continue
		case arg == "--apply":
			req.DryRun = false
		case arg == "--dry-run":
			req.DryRun = true
		case arg == "--kind":
			value, next, err := replayFlagValue(args, i, arg)
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
			}
			req.Kind = value
			i = next
		case strings.HasPrefix(arg, "--kind="):
			req.Kind = strings.TrimPrefix(arg, "--kind=")
		case arg == "--from":
			value, next, err := replayFlagValue(args, i, arg)
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
			}
			req.From, err = time.Parse(time.RFC3339, strings.TrimSpace(value))
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("invalid replay from time")
			}
			i = next
		case strings.HasPrefix(arg, "--from="):
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(strings.TrimPrefix(arg, "--from=")))
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("invalid replay from time")
			}
			req.From = parsed
		case arg == "--to":
			value, next, err := replayFlagValue(args, i, arg)
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
			}
			req.To, err = time.Parse(time.RFC3339, strings.TrimSpace(value))
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("invalid replay to time")
			}
			i = next
		case strings.HasPrefix(arg, "--to="):
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(strings.TrimPrefix(arg, "--to=")))
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("invalid replay to time")
			}
			req.To = parsed
		case arg == "--event-type":
			value, next, err := replayFlagValue(args, i, arg)
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
			}
			req.EventTypes = append(req.EventTypes, value)
			i = next
		case strings.HasPrefix(arg, "--event-type="):
			req.EventTypes = append(req.EventTypes, strings.TrimPrefix(arg, "--event-type="))
		case arg == "--event-types":
			value, next, err := replayFlagValue(args, i, arg)
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
			}
			req.EventTypes = append(req.EventTypes, splitReplayEventTypes(value)...)
			i = next
		case strings.HasPrefix(arg, "--event-types="):
			req.EventTypes = append(req.EventTypes, splitReplayEventTypes(strings.TrimPrefix(arg, "--event-types="))...)
		case arg == "--actor-id":
			value, next, err := replayFlagValue(args, i, arg)
			if err != nil {
				return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
			}
			actor.ID = strings.TrimSpace(value)
			i = next
		case strings.HasPrefix(arg, "--actor-id="):
			actor.ID = strings.TrimSpace(strings.TrimPrefix(arg, "--actor-id="))
		default:
			return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("unknown ops replay argument")
		}
	}
	if actor.ID == "" {
		return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, fmt.Errorf("ops replay actor id is required")
	}
	req, err := ticketing.ValidateReplayOutboxRequest(req)
	if err != nil {
		return ticketing.ReplayOutboxRequest{}, ticketing.Actor{}, err
	}
	return req, actor, nil
}

func replayFlagValue(args []string, index int, flag string) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("ops replay argument %q requires a value", flag)
	}
	return args[index+1], index + 1, nil
}

func splitReplayEventTypes(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, strings.TrimSpace(part))
	}
	return values
}
