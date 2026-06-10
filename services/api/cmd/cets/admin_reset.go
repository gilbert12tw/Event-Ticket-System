package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func resetDemoDB(cfg config.Config, logger *slog.Logger) error {
	if cfg.IsProduction() {
		return errors.New("reset-demo-db is disabled when APP_ENV=production")
	}
	redisClient, err := newResetRedisClient(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer closeRedisClient(logger, redisClient)

	return withDatabase(cfg, cfg.ValidateDatabase, func(ctx context.Context, pool *pgxpool.Pool) error {
		return resetDemoData(ctx, pool, redisClient, cfg, logger)
	})
}

func resetDemoData(ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, cfg config.Config, logger *slog.Logger) error {
	// Ensure the schema exists before truncating.
	if err := postgres.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("reset-demo-db: migrate schema: %w", err)
	}
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("reset-demo-db: redis ping failed: %w", err)
	}
	if err := truncateAppData(ctx, pool); err != nil {
		return fmt.Errorf("reset-demo-db: truncate app data: %w", err)
	}
	// TRUNCATE removes migration-seeded baseline rows (e.g.
	// reporting_projection_offsets); re-run migrations to restore them.
	if err := postgres.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("reset-demo-db: restore baseline rows: %w", err)
	}
	deletedKeys, err := clearRedisAppKeys(ctx, redisClient)
	if err != nil {
		return fmt.Errorf("reset-demo-db: clear redis keys: %w", err)
	}
	service := newTicketingService(pool, cfg, logger)
	if err := service.SeedDemoData(ctx); err != nil {
		return fmt.Errorf("reset-demo-db: seed demo data: %w", err)
	}
	seed, err := service.SeedDemoEventTickets(ctx)
	if err != nil {
		return fmt.Errorf("reset-demo-db: seed demo event tickets: %w", err)
	}
	logger.Info("demo database reset complete",
		"redis_keys_deleted", deletedKeys,
		"today_event_id", seed.TodayEventID,
		"today_ticket_count", len(seed.TodayTicketIDs),
		"future_event_id", seed.FutureEventID,
		"future_ticket_count", len(seed.FutureTicketIDs),
	)
	return nil
}

func newResetRedisClient(redisURL string) (*redis.Client, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, errors.New("REDIS_URL is required for reset-demo-db")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, errors.New("invalid REDIS_URL")
	}
	return redis.NewClient(opts), nil
}

func truncateAppData(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `TRUNCATE
		audit_logs,
		booking_bans,
		booking_idempotency_results,
		checkin_records,
		checkin_rejections,
		eligibility_impact_reviews,
		eligibility_rule_versions,
		eligibility_rules,
		employees,
		event_assets,
		event_versions,
		events,
		hr_sync_batches,
		lottery_results,
		lottery_runs,
		no_show_records,
		notification_deliveries,
		notification_preferences,
		offline_checkin_batches,
		offline_checkin_scans,
		outbox_events,
		registrations,
		report_exports,
		reporting_event_summary,
		reporting_projection_offsets,
		reservation_compensation_metrics,
		tickets
		RESTART IDENTITY CASCADE`)
	return err
}

func clearRedisAppKeys(ctx context.Context, client *redis.Client) (int64, error) {
	resetRedisPrefixes := []string{
		"cets:v1:resv:",
		"cets:v1:rate:booking:",
	}
	var deleted int64
	for _, prefix := range resetRedisPrefixes {
		iter := client.Scan(ctx, 0, prefix+"*", 100).Iterator()
		var batch []string
		for iter.Next(ctx) {
			batch = append(batch, iter.Val())
			if len(batch) == 100 {
				count, err := deleteRedisBatch(ctx, client, batch)
				if err != nil {
					return deleted, err
				}
				deleted += count
				batch = batch[:0]
			}
		}
		if err := iter.Err(); err != nil {
			return deleted, err
		}
		count, err := deleteRedisBatch(ctx, client, batch)
		if err != nil {
			return deleted, err
		}
		deleted += count
	}
	return deleted, nil
}

func deleteRedisBatch(ctx context.Context, client *redis.Client, keys []string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	return client.Del(ctx, keys...).Result()
}
