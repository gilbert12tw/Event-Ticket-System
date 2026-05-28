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
	"math"
	"os"
	"sort"
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

type result struct {
	Mode             string    `json:"mode"`
	VUs              int       `json:"vus"`
	Capacity         int       `json:"capacity"`
	WallClockMS      float64   `json:"wall_clock_ms"`
	RPS              float64   `json:"rps"`
	Outcomes         outcomes  `json:"outcomes"`
	LatencyMS        latencies `json:"latency_ms"`
	DBConfirmedCount int       `json:"db_confirmed_count"`
}

type outcomes struct {
	Confirmed  int `json:"confirmed"`
	Waitlisted int `json:"waitlisted"`
	Error      int `json:"error"`
}

type latencies struct {
	Min  float64   `json:"min"`
	P50  float64   `json:"p50"`
	P90  float64   `json:"p90"`
	P95  float64   `json:"p95"`
	P99  float64   `json:"p99"`
	Max  float64   `json:"max"`
	Mean float64   `json:"mean"`
	All  []float64 `json:"all_ms"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "experiment failed:", err)
		os.Exit(1)
	}
}

func run() error {
	mode := getenv("EXPERIMENT_MODE", "off")
	if mode != "off" && mode != "on" {
		return fmt.Errorf("EXPERIMENT_MODE must be 'off' or 'on', got %q", mode)
	}
	vus := atoiOr("EXPERIMENT_VUS", 200)
	capacity := atoiOr("EXPERIMENT_CAPACITY", 10)
	if vus <= 0 || capacity <= 0 {
		return errors.New("EXPERIMENT_VUS and EXPERIMENT_CAPACITY must be positive")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := postgres.Connect(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("postgres connect: %w", err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	service := ticketing.NewServiceWithPolicy(pool, ticketing.NewSigner(signerSecret), logger, ticketing.NoShowPolicy{})

	var redisClient *redis.Client
	if mode == "on" {
		redisURL := os.Getenv("REDIS_URL")
		if redisURL == "" {
			return errors.New("REDIS_URL is required when EXPERIMENT_MODE=on")
		}
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			return fmt.Errorf("parse REDIS_URL: %w", err)
		}
		redisClient = redis.NewClient(opts)
		defer redisClient.Close()
		if err := flushReservationKeys(ctx, redisClient); err != nil {
			return fmt.Errorf("flush redis: %w", err)
		}
		gate := reservation.NewRedisGate(redisClient, reservation.Config{
			Enabled:          true,
			OutageMode:       reservation.OutageModeDegrade,
			HashSecret:       []byte(experimentSecret),
			TTL:              20 * time.Second,
			OperationTimeout: 150 * time.Millisecond,
		}, logger)
		service.WithReservationGate(gate, []byte(experimentSecret))
	}

	if err := resetSchema(ctx, pool); err != nil {
		return fmt.Errorf("reset schema: %w", err)
	}
	eventID, err := seedFixtures(ctx, pool, service, vus, capacity)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if err := warmup(ctx, pool, service, eventID, redisClient); err != nil {
		return fmt.Errorf("warmup: %w", err)
	}

	latenciesMs := make([]float64, vus)
	var confirmedCount, waitlistedCount, errCount atomic.Int64

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(vus)
	for i := 0; i < vus; i++ {
		i := i
		go func() {
			defer wg.Done()
			employeeID := employeeIDFor(i)
			req := ticketing.BookingRequest{EmployeeID: employeeID, IdempotencyKey: fmt.Sprintf("exp-%s-%d", mode, i)}
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
	wall := time.Since(start)

	var dbConfirmed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, eventID).Scan(&dbConfirmed); err != nil {
		return fmt.Errorf("verify dbConfirmed: %w", err)
	}

	out := result{
		Mode:        mode,
		VUs:         vus,
		Capacity:    capacity,
		WallClockMS: float64(wall.Microseconds()) / 1000.0,
		RPS:         float64(vus) / wall.Seconds(),
		Outcomes: outcomes{
			Confirmed:  int(confirmedCount.Load()),
			Waitlisted: int(waitlistedCount.Load()),
			Error:      int(errCount.Load()),
		},
		LatencyMS:        summarize(latenciesMs),
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
	_, _ = service.Book(ctx,
		ticketing.Actor{ID: warmupEmployee, Role: ticketing.RoleEmployee},
		eventID,
		ticketing.BookingRequest{EmployeeID: warmupEmployee, IdempotencyKey: "warmup-key"})
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

func summarize(xs []float64) latencies {
	if len(xs) == 0 {
		return latencies{}
	}
	cp := make([]float64, len(xs))
	copy(cp, xs)
	sort.Float64s(cp)
	sum := 0.0
	for _, v := range cp {
		sum += v
	}
	return latencies{
		Min:  cp[0],
		P50:  percentile(cp, 0.50),
		P90:  percentile(cp, 0.90),
		P95:  percentile(cp, 0.95),
		P99:  percentile(cp, 0.99),
		Max:  cp[len(cp)-1],
		Mean: sum / float64(len(cp)),
		All:  cp,
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
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
