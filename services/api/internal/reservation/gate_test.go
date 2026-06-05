package reservation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// newTestGate returns a RedisGate connected to the URL in REDIS_URL. Tests
// that depend on real Redis behavior skip when REDIS_URL is empty so the
// default `go test` invocation does not require Compose to be running. The
// Phase 2 live-gates CI job runs with Redis available and exercises this
// path end-to-end.
func newTestGate(t *testing.T) (*RedisGate, redis.UniversalClient, func()) {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set; skipping reservation gate integration test")
	}
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	require.NoError(t, client.Ping(context.Background()).Err())
	gate := NewRedisGate(client, Config{
		Enabled:          true,
		OutageMode:       OutageModeDegrade,
		HashSecret:       []byte("test-secret-do-not-use-in-prod"),
		TTL:              5 * time.Second,
		OperationTimeout: 500 * time.Millisecond,
	}, nil)
	return gate, client, func() {
		_ = client.Close()
	}
}

func uniqueEventID(t *testing.T) string {
	t.Helper()
	id, err := newReservationID()
	require.NoError(t, err)
	return "evt_" + id
}

func TestTraceRedisScriptUsesServiceGraphAttributes(t *testing.T) {
	exporter, shutdown := installReservationTraceExporter(t)
	defer shutdown()

	result, err := traceRedisScript(context.Background(), "reserve", nil, func(context.Context) (interface{}, error) {
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "redis.reserve", spans[0].Name)
	assert.Contains(t, spans[0].Attributes, attribute.String("peer.service", "redis"))
	assert.Contains(t, spans[0].Attributes, attribute.String("server.address", "redis"))
	for _, attr := range spans[0].Attributes {
		value := attr.Value.AsString()
		assert.NotContains(t, value, "idempotency")
		assert.NotContains(t, value, "email@example.test")
		assert.NotContains(t, value, "token")
	}
}

func installReservationTraceExporter(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	return exporter, func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	}
}

func TestRedisGateReserveGrantsThenExhausts(t *testing.T) {
	gate, client, cleanup := newTestGate(t)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), versionKey(eventID))

	probe := func(ctx context.Context) (int, int64, error) { return 2, 1, nil }
	for i := 0; i < 2; i++ {
		hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "key-"+string(rune('A'+i)))
		hold, err := gate.Reserve(ctx, eventID, hash, "actor-1", probe)
		require.NoError(t, err)
		require.Equal(t, OutcomeGranted, hold.Outcome, "attempt %d should be granted", i)
		defer client.Del(ctx, holdKey(eventID, hash))
	}
	exhaustedHash := Hash([]byte("k"), "registration.book", eventID, "E1099", "exhausted")
	defer client.Del(ctx, holdKey(eventID, exhaustedHash))
	hold, err := gate.Reserve(ctx, eventID, exhaustedHash, "actor-2", probe)
	require.NoError(t, err)
	require.Equal(t, OutcomeExhausted, hold.Outcome)
}

func TestRedisGateReserveProbesOnlyWhenCounterMissing(t *testing.T) {
	gate, client, cleanup := newTestGate(t)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), versionKey(eventID))

	probeCalls := 0
	probe := func(ctx context.Context) (int, int64, error) {
		probeCalls++
		return 3, 7, nil
	}
	for i := 0; i < 3; i++ {
		hash := Hash([]byte("k"), "registration.book", eventID, "E100"+string(rune('1'+i)), "key-"+string(rune('A'+i)))
		defer client.Del(ctx, holdKey(eventID, hash))
		hold, err := gate.Reserve(ctx, eventID, hash, "actor-1", probe)
		require.NoError(t, err)
		require.Equal(t, OutcomeGranted, hold.Outcome)
		require.Equal(t, int64(7), hold.CapacityVersion)
	}
	require.Equal(t, 1, probeCalls, "probe should only seed a missing Redis counter")
}

func TestRedisGateDuplicateReserveReturnsExistingHold(t *testing.T) {
	gate, client, cleanup := newTestGate(t)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "same-key")
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), versionKey(eventID), holdKey(eventID, hash))

	probe := func(ctx context.Context) (int, int64, error) { return 5, 1, nil }
	first, err := gate.Reserve(ctx, eventID, hash, "actor-1", probe)
	require.NoError(t, err)
	require.Equal(t, OutcomeGranted, first.Outcome)

	second, err := gate.Reserve(ctx, eventID, hash, "actor-1", probe)
	require.NoError(t, err)
	require.Equal(t, OutcomeDuplicate, second.Outcome)
	require.Equal(t, first.ReservationID, second.ReservationID)

	// Counter must not have decremented twice.
	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	require.Equal(t, 4, rem)
}

func TestRedisGateReleaseReturnsSlot(t *testing.T) {
	gate, client, cleanup := newTestGate(t)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "release-key")
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), versionKey(eventID), holdKey(eventID, hash))

	probe := func(ctx context.Context) (int, int64, error) { return 1, 1, nil }
	_, err := gate.Reserve(ctx, eventID, hash, "actor", probe)
	require.NoError(t, err)
	require.NoError(t, gate.Release(ctx, eventID, hash))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	require.Equal(t, 1, rem, "release must return the advisory slot")

	// Re-reserving with a new key after release should succeed.
	otherHash := Hash([]byte("k"), "registration.book", eventID, "E1002", "second")
	defer client.Del(ctx, holdKey(eventID, otherHash))
	hold, err := gate.Reserve(ctx, eventID, otherHash, "actor-2", probe)
	require.NoError(t, err)
	require.Equal(t, OutcomeGranted, hold.Outcome)
}

func TestRedisGateConfirmDoesNotIncrementCounter(t *testing.T) {
	gate, client, cleanup := newTestGate(t)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "confirm-key")
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), versionKey(eventID), holdKey(eventID, hash))

	probe := func(ctx context.Context) (int, int64, error) { return 3, 1, nil }
	_, err := gate.Reserve(ctx, eventID, hash, "actor", probe)
	require.NoError(t, err)
	require.NoError(t, gate.Confirm(ctx, eventID, hash))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	require.Equal(t, 2, rem, "confirm must remove the hold without returning the slot")
	exists, err := client.Exists(ctx, holdKey(eventID, hash)).Result()
	require.NoError(t, err)
	require.Equal(t, int64(0), exists)
}

func TestRedisGatePressureSnapshotCountsActiveHolds(t *testing.T) {
	gate, client, cleanup := newTestGate(t)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "pressure-key")
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), holdKey(eventID, hash))

	probe := func(ctx context.Context) (int, int64, error) { return 2, 1, nil }
	_, err := gate.Reserve(ctx, eventID, hash, "actor", probe)
	require.NoError(t, err)

	snapshot, err := gate.PressureSnapshot(ctx, eventID)

	require.NoError(t, err)
	require.Equal(t, PressureStateAvailable, snapshot.State)
	require.Equal(t, 1, snapshot.ActiveCount)
}

func TestRedisGatePressureSnapshotsCountActiveHoldsInOneBatch(t *testing.T) {
	gate, client, cleanup := newTestGate(t)
	defer cleanup()
	ctx := context.Background()
	firstEventID := uniqueEventID(t)
	secondEventID := uniqueEventID(t)
	hash := Hash([]byte("k"), "registration.book", firstEventID, "E1001", "pressure-batch-key")
	defer client.Del(ctx,
		remainingKey(firstEventID),
		pendingKey(firstEventID),
		holdKey(firstEventID, hash),
		remainingKey(secondEventID),
		pendingKey(secondEventID),
	)

	probe := func(ctx context.Context) (int, int64, error) { return 2, 1, nil }
	_, err := gate.Reserve(ctx, firstEventID, hash, "actor", probe)
	require.NoError(t, err)

	snapshots, err := gate.PressureSnapshots(ctx, []string{firstEventID, secondEventID})

	require.NoError(t, err)
	require.Equal(t, PressureStateAvailable, snapshots[firstEventID].State)
	require.Equal(t, 1, snapshots[firstEventID].ActiveCount)
	require.Equal(t, PressureStateAvailable, snapshots[secondEventID].State)
	require.Equal(t, 0, snapshots[secondEventID].ActiveCount)
}

func TestRedisGateExhaustedDoesNotDecrementBelowZero(t *testing.T) {
	gate, client, cleanup := newTestGate(t)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), versionKey(eventID))

	probe := func(ctx context.Context) (int, int64, error) { return 0, 1, nil }
	for i := 0; i < 5; i++ {
		hash := Hash([]byte("k"), "registration.book", eventID, "E", "k"+string(rune('0'+i)))
		defer client.Del(ctx, holdKey(eventID, hash))
		hold, err := gate.Reserve(ctx, eventID, hash, "actor", probe)
		require.NoError(t, err)
		require.Equal(t, OutcomeExhausted, hold.Outcome)
	}
	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	require.GreaterOrEqual(t, rem, 0, "counter must never go negative")
}

func TestNoopGateAlwaysGrants(t *testing.T) {
	g := NoopGate{}
	require.False(t, g.Enabled())
	probe := func(ctx context.Context) (int, int64, error) { return 0, 0, nil }
	hold, err := g.Reserve(context.Background(), "evt", "h", "a", probe)
	require.NoError(t, err)
	require.Equal(t, OutcomeGranted, hold.Outcome)
	snapshot, err := g.PressureSnapshot(context.Background(), "evt")
	require.NoError(t, err)
	require.Equal(t, PressureStateDisabled, snapshot.State)
	snapshots, err := g.PressureSnapshots(context.Background(), []string{"evt_a", "evt_b"})
	require.NoError(t, err)
	require.Equal(t, PressureStateDisabled, snapshots["evt_a"].State)
	require.Equal(t, PressureStateDisabled, snapshots["evt_b"].State)
}

func TestHashIsDeterministicAndDomainSeparated(t *testing.T) {
	secret := []byte("k")
	a := Hash(secret, "registration.book", "evt", "E", "key")
	b := Hash(secret, "registration.book", "evt", "E", "key")
	require.Equal(t, a, b, "deterministic")
	require.NotEqual(t, a, Hash(secret, "registration.cancel", "evt", "E", "key"), "operation must scope")
	require.NotEqual(t, a, Hash(secret, "registration.book", "other", "E", "key"), "event must scope")
	require.NotEqual(t, a, Hash(secret, "registration.book", "evt", "E2", "key"), "actor must scope")
	require.NotEqual(t, a, Hash(secret, "registration.book", "evt", "E", "key2"), "idempotency key must scope")
}

func TestConfigValidate(t *testing.T) {
	t.Run("disabled gate skips validation", func(t *testing.T) {
		require.NoError(t, Config{}.Validate())
	})
	t.Run("enabled requires secret", func(t *testing.T) {
		err := Config{Enabled: true, TTL: time.Second, OperationTimeout: time.Millisecond, OutageMode: OutageModeDegrade}.Validate()
		require.Error(t, err)
	})
	t.Run("enabled requires positive ttl", func(t *testing.T) {
		err := Config{Enabled: true, HashSecret: []byte("k"), OperationTimeout: time.Millisecond, OutageMode: OutageModeDegrade}.Validate()
		require.Error(t, err)
	})
}
