// Command reservation_experiment drives N concurrent booking attempts at a
// single hot limited event and prints a JSON summary of the run. It is the
// measurement harness for the PH2-22 Redis reservation gate report.
// PostgreSQL remains the source of truth; the gate only changes the booking
// path's scheduling behavior, never the committed counts.
//
// Reads env:
//
//	EXPERIMENT_MODE=off|on  toggles BOOKING_PREADMISSION
//	EXPERIMENT_VUS          concurrent booking attempts (default 200)
//	EXPERIMENT_CAPACITY     limited event capacity (default 10)
//	EXPERIMENT_TIMEOUT_SECONDS
//	                        whole-run timeout (default 120)
//	EXPERIMENT_ALLOW_DESTRUCTIVE=1
//	                        required because the harness truncates tables and
//	                        deletes reservation keys
//	DATABASE_URL            Postgres connection string
//	REDIS_URL               Redis connection string (required when mode=on)
//
// Writes JSON to stdout, logs to stderr.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/reservation"
	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	experimentSecret = "experiment-hash-secret"
	signerSecret     = "experiment-signer-secret"
	warmupEmployee   = "EXP_WARM_0001"
	hotEventTitle    = "Redis Gate Experiment Hot Event"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "experiment failed:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadExperimentConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	pool, service, redisClient, err := setupExperimentRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if redisClient != nil {
		defer func() { _ = redisClient.Close() }()
	}

	eventID, err := prepareExperimentData(ctx, pool, service, cfg, redisClient)
	if err != nil {
		return err
	}
	runResult := runBookingBurst(ctx, service, eventID, cfg)
	dbConfirmed, err := experimentDBConfirmedCount(ctx, pool, eventID)
	if err != nil {
		return err
	}
	if err := validateExperimentOutcome(cfg.VUs, cfg.Capacity, runResult.confirmed, runResult.waitlisted, runResult.errors, dbConfirmed); err != nil {
		return err
	}
	return writeExperimentResult(cfg, runResult, dbConfirmed)
}

type experimentConfig struct {
	Mode     string
	VUs      int
	Capacity int
	Timeout  time.Duration
	DBURL    string
}

type bookingBurstResult struct {
	latenciesMs []float64
	wall        time.Duration
	confirmed   int
	waitlisted  int
	errors      int
}

func loadExperimentConfig() (experimentConfig, error) {
	cfg := experimentConfig{
		Mode:     getenv("EXPERIMENT_MODE", "off"),
		VUs:      atoiOr("EXPERIMENT_VUS", 200),
		Capacity: atoiOr("EXPERIMENT_CAPACITY", 10),
		Timeout:  time.Duration(atoiOr("EXPERIMENT_TIMEOUT_SECONDS", 120)) * time.Second,
		DBURL:    os.Getenv("DATABASE_URL"),
	}
	if cfg.Mode != "off" && cfg.Mode != "on" {
		return experimentConfig{}, fmt.Errorf("EXPERIMENT_MODE must be 'off' or 'on', got %q", cfg.Mode)
	}
	if cfg.DBURL == "" {
		return experimentConfig{}, errors.New("DATABASE_URL is required")
	}
	if err := validateExperimentShape(cfg.VUs, cfg.Capacity); err != nil {
		return experimentConfig{}, err
	}
	if err := requireDestructiveOptIn(); err != nil {
		return experimentConfig{}, err
	}
	return cfg, nil
}

func setupExperimentRuntime(ctx context.Context, cfg experimentConfig) (*pgxpool.Pool, *ticketing.Service, *redis.Client, error) {
	pool, err := postgres.Connect(ctx, cfg.DBURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("postgres connect: %w", err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, nil, fmt.Errorf("migrate: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	service := ticketing.NewServiceWithPolicy(pool, ticketing.NewSigner(signerSecret), logger, ticketing.NoShowPolicy{})
	redisClient, err := configureExperimentGate(ctx, cfg, logger, service)
	if err != nil {
		pool.Close()
		return nil, nil, nil, err
	}
	return pool, service, redisClient, nil
}

func configureExperimentGate(ctx context.Context, cfg experimentConfig, logger *slog.Logger, service *ticketing.Service) (*redis.Client, error) {
	if cfg.Mode != "on" {
		return nil, nil
	}
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return nil, errors.New("REDIS_URL is required when EXPERIMENT_MODE=on")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, errors.New("REDIS_URL is invalid")
	}
	redisClient := redis.NewClient(opts)
	if err := flushReservationKeys(ctx, redisClient); err != nil {
		_ = redisClient.Close()
		return nil, fmt.Errorf("flush redis: %w", err)
	}
	gate := reservation.NewRedisGate(redisClient, reservation.Config{
		Enabled:          true,
		OutageMode:       reservation.OutageModeDegrade,
		HashSecret:       []byte(experimentSecret),
		TTL:              20 * time.Second,
		OperationTimeout: 150 * time.Millisecond,
	}, logger)
	service.WithReservationGate(gate, []byte(experimentSecret))
	return redisClient, nil
}

func prepareExperimentData(ctx context.Context, pool *pgxpool.Pool, service *ticketing.Service, cfg experimentConfig, redisClient *redis.Client) (string, error) {
	if err := resetSchema(ctx, pool); err != nil {
		return "", fmt.Errorf("reset schema: %w", err)
	}
	eventID, err := seedFixtures(ctx, pool, service, cfg.VUs, cfg.Capacity)
	if err != nil {
		return "", fmt.Errorf("seed: %w", err)
	}
	if err := warmup(ctx, pool, service, eventID, redisClient); err != nil {
		return "", fmt.Errorf("warmup: %w", err)
	}
	return eventID, nil
}

func runBookingBurst(ctx context.Context, service *ticketing.Service, eventID string, cfg experimentConfig) bookingBurstResult {
	latenciesMs := make([]float64, cfg.VUs)
	var confirmedCount, waitlistedCount, errCount atomic.Int64

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(cfg.VUs)
	for i := 0; i < cfg.VUs; i++ {
		i := i
		go func() {
			defer wg.Done()
			employeeID := employeeIDFor(i)
			req := ticketing.BookingRequest{EmployeeID: employeeID, IdempotencyKey: fmt.Sprintf("exp-%s-%d", cfg.Mode, i)}
			t0 := time.Now()
			res, err := service.Book(ctx, ticketing.Actor{ID: employeeID, Role: ticketing.RoleEmployee}, eventID, req)
			latenciesMs[i] = float64(time.Since(t0).Microseconds()) / 1000.0
			if err != nil {
				errCount.Add(1)
				return
			}
			switch res.Registration.Status {
			case ticketing.RegistrationConfirmed:
				confirmedCount.Add(1)
			case ticketing.RegistrationWaitlisted:
				waitlistedCount.Add(1)
			}
		}()
	}
	wg.Wait()
	return bookingBurstResult{
		latenciesMs: latenciesMs,
		wall:        time.Since(start),
		confirmed:   int(confirmedCount.Load()),
		waitlisted:  int(waitlistedCount.Load()),
		errors:      int(errCount.Load()),
	}
}

func experimentDBConfirmedCount(ctx context.Context, pool *pgxpool.Pool, eventID string) (int, error) {
	var dbConfirmed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, eventID).Scan(&dbConfirmed); err != nil {
		return 0, fmt.Errorf("verify dbConfirmed: %w", err)
	}
	return dbConfirmed, nil
}

func writeExperimentResult(cfg experimentConfig, runResult bookingBurstResult, dbConfirmed int) error {
	out := result{
		Mode:        cfg.Mode,
		VUs:         cfg.VUs,
		Capacity:    cfg.Capacity,
		WallClockMS: float64(runResult.wall.Microseconds()) / 1000.0,
		RPS:         float64(cfg.VUs) / runResult.wall.Seconds(),
		Outcomes: outcomes{
			Confirmed:  runResult.confirmed,
			Waitlisted: runResult.waitlisted,
			Error:      runResult.errors,
		},
		LatencyMS:        summarize(runResult.latenciesMs),
		DBConfirmedCount: dbConfirmed,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func resetSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `TRUNCATE outbox_events, audit_logs, checkin_records, tickets, registrations, booking_idempotency_results, eligibility_rules, events, employees RESTART IDENTITY CASCADE`)
	return err
}

func seedFixtures(ctx context.Context, pool *pgxpool.Pool, service *ticketing.Service, vus, capacity int) (string, error) {
	for i := 0; i < vus; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1,$2,'Engineering','Taipei HQ',6,'active')
			ON CONFLICT (employee_id) DO UPDATE SET full_name = EXCLUDED.full_name`,
			employeeIDFor(i), fmt.Sprintf("Exp Hot %05d", i))
		if err != nil {
			return "", err
		}
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
		VALUES ($1,'Experiment Warmup','Engineering','Taipei HQ',6,'active')
		ON CONFLICT (employee_id) DO NOTHING`, warmupEmployee)
	if err != nil {
		return "", err
	}

	admin := ticketing.Actor{ID: "experiment-admin", Role: ticketing.RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, ticketing.CreateEventRequest{
		Title:    hotEventTitle,
		Capacity: capacity,
		Status:   ticketing.EventStatusPublished,
		Rule: ticketing.RuleInput{
			Department:       "Engineering",
			Site:             "Taipei HQ",
			MinGrade:         5,
			EmploymentStatus: "active",
		},
	})
	if err != nil {
		return "", err
	}
	return event.EventID, nil
}

func warmup(ctx context.Context, pool *pgxpool.Pool, service *ticketing.Service, eventID string, redisClient *redis.Client) error {
	if _, err := service.Book(ctx,
		ticketing.Actor{ID: warmupEmployee, Role: ticketing.RoleEmployee},
		eventID,
		ticketing.BookingRequest{EmployeeID: warmupEmployee, IdempotencyKey: "warmup-key"}); err != nil {
		return fmt.Errorf("book warmup: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tickets WHERE registration_id IN (SELECT registration_id FROM registrations WHERE employee_id = $1)`, warmupEmployee); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM registrations WHERE employee_id = $1`, warmupEmployee); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM outbox_events WHERE event_type IN ('booking.confirmed','booking.waitlisted')`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_logs`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM booking_idempotency_results`); err != nil {
		return err
	}
	if redisClient != nil {
		if err := flushReservationKeys(ctx, redisClient); err != nil {
			return err
		}
	}
	return nil
}

func flushReservationKeys(ctx context.Context, client *redis.Client) error {
	keys, err := client.Keys(ctx, "cets:v1:resv:*").Result()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return client.Del(ctx, keys...).Err()
}

func employeeIDFor(i int) string { return fmt.Sprintf("EXP%05d", i) }

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func atoiOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
