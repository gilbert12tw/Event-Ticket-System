package reservation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLookup is a hand-rolled BookingLookup so the compensator tests can
// run against a real Redis without dragging Postgres into the package
// boundary. The PR3 worker test exercises a real DB-backed BookingLookup.
type fakeLookup struct {
	mu        sync.Mutex
	bookings  map[string]BookingStatus // key = eventID + "|" + hash
	capacity  map[string]int           // key = eventID
	probeErrs int
	probeErr  error
}

func newFakeLookup() *fakeLookup {
	return &fakeLookup{
		bookings: map[string]BookingStatus{},
		capacity: map[string]int{},
	}
}

func (f *fakeLookup) set(eventID, hash string, st BookingStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bookings[eventID+"|"+hash] = st
}

func (f *fakeLookup) setCapacity(eventID string, capacity int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.capacity[eventID] = capacity
}

func (f *fakeLookup) BookingByHash(_ context.Context, eventID, hash string) (BookingStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bookings[eventID+"|"+hash], nil
}

func (f *fakeLookup) RemainingCapacity(_ context.Context, eventID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.probeErrs > 0 {
		f.probeErrs--
		return 0, f.probeErr
	}
	return f.capacity[eventID], nil
}

func newTestCompensator(t *testing.T, lookup BookingLookup) (*Compensator, *RedisGate, redis.UniversalClient, func()) {
	t.Helper()
	gate, client, cleanup := newTestGate(t)
	comp := NewCompensator(client, CompensationConfig{
		GraceTTL:         100 * time.Millisecond,
		BatchSize:        16,
		MaxEvents:        16,
		DriftMarkerTTL:   5 * time.Second,
		OperationTimeout: 500 * time.Millisecond,
	}, lookup, nil, nil)
	return comp, gate, client, cleanup
}

// seedHold registers a pending hold via the production Reserve path so the
// Redis state matches what the application would produce; this exercises
// the same Lua surface the compensator must reconcile.
func seedHold(t *testing.T, gate *RedisGate, eventID, hash string, capacity int) {
	t.Helper()
	probe := func(_ context.Context) (int, int64, error) { return capacity, 1, nil }
	hold, err := gate.Reserve(context.Background(), eventID, hash, "actor", probe)
	require.NoError(t, err)
	require.Equal(t, OutcomeGranted, hold.Outcome)
}

// expirePending backdates the pending-set score for an existing hold so the
// compensator's `now - grace` window picks it up. Production code never
// rewrites pending scores; this is the test-only shortcut equivalent to
// "wait for the TTL grace to elapse" without actually sleeping seconds.
func expirePending(t *testing.T, client redis.UniversalClient, eventID, hash string) {
	t.Helper()
	past := time.Now().Add(-1 * time.Hour).Unix()
	require.NoError(t, client.ZAdd(context.Background(), pendingKey(eventID),
		redis.Z{Score: float64(past), Member: hash}).Err())
}

func TestCompensatorReleasesOrphanedHoldWhenNoDBRow(t *testing.T) {
	lookup := newFakeLookup()
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "k1")
	defer client.Del(ctx, holdKey(eventID, hash))

	lookup.setCapacity(eventID, 3)
	seedHold(t, gate, eventID, hash, 3) // counter: 3 -> 2 (decremented)
	expirePending(t, client, eventID, hash)

	require.NoError(t, comp.Sweep(ctx))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 3, rem, "slot must be returned after release")
	exists, err := client.Exists(ctx, holdKey(eventID, hash)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "hold must be removed")
	pendingCount, err := client.ZCard(ctx, pendingKey(eventID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), pendingCount, "pending member must be cleared")
}

func TestCompensatorDropsHoldWhenBookingConfirmed(t *testing.T) {
	lookup := newFakeLookup()
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "confirmed-k")
	defer client.Del(ctx, holdKey(eventID, hash))

	lookup.setCapacity(eventID, 2)
	seedHold(t, gate, eventID, hash, 3) // counter: 3 -> 2 (one seat consumed by confirmed booking)
	lookup.set(eventID, hash, BookingStatus{Found: true, Completed: true, Confirmed: true})
	expirePending(t, client, eventID, hash)

	require.NoError(t, comp.Sweep(ctx))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 2, rem, "confirmed bookings own their slot; counter must NOT increment")
	exists, err := client.Exists(ctx, holdKey(eventID, hash)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func TestCompensatorReleasesHoldWhenBookingWaitlisted(t *testing.T) {
	lookup := newFakeLookup()
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "waitlist-k")
	defer client.Del(ctx, holdKey(eventID, hash))

	lookup.setCapacity(eventID, 3)
	seedHold(t, gate, eventID, hash, 3)
	// Waitlisted = completed but not confirmed: waitlist doesn't consume
	// capacity, so the advisory slot must come back.
	lookup.set(eventID, hash, BookingStatus{Found: true, Completed: true, Confirmed: false})
	expirePending(t, client, eventID, hash)

	require.NoError(t, comp.Sweep(ctx))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 3, rem, "waitlisted booking does not own the slot; counter must be restored")
}

func TestCompensatorSkipsMembersWithinGracePeriod(t *testing.T) {
	lookup := newFakeLookup()
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "fresh-k")
	defer client.Del(ctx, holdKey(eventID, hash))

	lookup.setCapacity(eventID, 3)
	seedHold(t, gate, eventID, hash, 3)
	// Do NOT wait for grace — the booking is still in flight.

	require.NoError(t, comp.Sweep(ctx))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 2, rem, "in-flight booking must not be reclaimed prematurely")
	exists, err := client.Exists(ctx, holdKey(eventID, hash)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists, "in-flight hold must remain")
}

// Regression (HIGH): a booking_idempotency_results row that exists but is not
// yet completed marks an in-flight booking (e.g. stalled on lock contention
// past the grace TTL). Releasing its hold would hand the slot to a second
// request while the first may still confirm, defeating the gate. The sweep
// must skip it — keeping both the hold and the pending member — so a later
// sweep re-evaluates once the row settles.
func TestCompensatorSkipsInFlightBookingPastGrace(t *testing.T) {
	lookup := newFakeLookup()
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "inflight-k")
	defer client.Del(ctx, holdKey(eventID, hash))

	lookup.setCapacity(eventID, 3)
	seedHold(t, gate, eventID, hash, 3) // counter: 3 -> 2
	lookup.set(eventID, hash, BookingStatus{Found: true, Completed: false})
	expirePending(t, client, eventID, hash)

	require.NoError(t, comp.Sweep(ctx))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 2, rem, "in-flight booking still owns its slot; counter must NOT be restored")
	pendingCount, err := client.ZCard(ctx, pendingKey(eventID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pendingCount, "pending member must remain for a later sweep to re-evaluate")

	// Once the booking settles as confirmed, the next sweep drops the hold
	// without returning the slot.
	lookup.set(eventID, hash, BookingStatus{Found: true, Completed: true, Confirmed: true})
	require.NoError(t, comp.Sweep(ctx))
	rem, err = client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 2, rem, "confirmed booking keeps the slot")
	pendingCount, err = client.ZCard(ctx, pendingKey(eventID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), pendingCount, "settled booking is reconciled and removed from pending")
}

func TestCompensatorReconciliationIsIdempotent(t *testing.T) {
	lookup := newFakeLookup()
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "idem-k")
	defer client.Del(ctx, holdKey(eventID, hash))

	lookup.setCapacity(eventID, 3)
	seedHold(t, gate, eventID, hash, 3)
	expirePending(t, client, eventID, hash)

	require.NoError(t, comp.Sweep(ctx))
	require.NoError(t, comp.Sweep(ctx)) // second sweep is a no-op

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 3, rem, "second sweep must not double-increment")
}

func TestCompensatorCapsCounterWhenAboveDBCapacity(t *testing.T) {
	lookup := newFakeLookup()
	comp, _, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))

	// Simulate drift: counter inflated above DB-derived capacity.
	require.NoError(t, client.Set(ctx, remainingKey(eventID), 999, 0).Err())
	lookup.setCapacity(eventID, 10)

	require.NoError(t, comp.SweepEvent(ctx, eventID))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 10, rem, "counter must be capped to DB-derived remaining capacity")

	marker, err := client.Get(ctx, driftKey(eventID)).Result()
	require.NoError(t, err)
	assert.Equal(t, "989", marker, "drift marker records the magnitude that was capped")
}

func TestCompensatorLeavesCounterAloneWhenNoDrift(t *testing.T) {
	lookup := newFakeLookup()
	comp, _, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))

	require.NoError(t, client.Set(ctx, remainingKey(eventID), 5, 0).Err())
	lookup.setCapacity(eventID, 10)

	require.NoError(t, comp.SweepEvent(ctx, eventID))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 5, rem)
	exists, err := client.Exists(ctx, driftKey(eventID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "no drift marker when nothing was capped")
}

func TestCompensatorReturnsSlotWhenHoldTTLExpired(t *testing.T) {
	lookup := newFakeLookup()
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "ttl-expired-k")
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), holdKey(eventID, hash), driftKey(eventID))

	lookup.setCapacity(eventID, 3)
	seedHold(t, gate, eventID, hash, 3) // counter: 3 -> 2
	// Simulate natural TTL expiry: Redis deletes the hold hash, but the
	// pending zset member remains for compensation to process.
	require.NoError(t, client.Del(ctx, holdKey(eventID, hash)).Err())
	expirePending(t, client, eventID, hash)

	require.NoError(t, comp.SweepEvent(ctx, eventID))

	rem, err := client.Get(ctx, remainingKey(eventID)).Int()
	require.NoError(t, err)
	assert.Equal(t, 3, rem, "TTL removes only the hold; compensation must return the advisory slot")
	count, err := client.ZCard(ctx, pendingKey(eventID)).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "orphan pending member must be removed")
}

func TestCompensatorScansMultipleEvents(t *testing.T) {
	lookup := newFakeLookup()
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()

	events := []string{uniqueEventID(t), uniqueEventID(t), uniqueEventID(t)}
	for _, eventID := range events {
		hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "k-"+eventID)
		t.Cleanup(func() {
			client.Del(ctx, remainingKey(eventID), pendingKey(eventID), holdKey(eventID, hash), driftKey(eventID))
		})
		lookup.setCapacity(eventID, 2)
		seedHold(t, gate, eventID, hash, 2)
		expirePending(t, client, eventID, hash)
	}

	require.NoError(t, comp.Sweep(ctx))

	for _, eventID := range events {
		rem, err := client.Get(ctx, remainingKey(eventID)).Int()
		require.NoError(t, err, "event %s missing counter", eventID)
		assert.Equal(t, 2, rem, "event %s slot must be returned", eventID)
	}
}

func TestCompensatorPropagatesLookupErrors(t *testing.T) {
	lookup := newFakeLookup()
	lookup.probeErrs = 1
	lookup.probeErr = errors.New("db connection refused")
	comp, gate, client, cleanup := newTestCompensator(t, lookup)
	defer cleanup()
	ctx := context.Background()
	eventID := uniqueEventID(t)
	defer client.Del(ctx, remainingKey(eventID), pendingKey(eventID), driftKey(eventID))
	hash := Hash([]byte("k"), "registration.book", eventID, "E1001", "err-k")
	defer client.Del(ctx, holdKey(eventID, hash))

	lookup.setCapacity(eventID, 3)
	seedHold(t, gate, eventID, hash, 3)
	expirePending(t, client, eventID, hash)

	// The cap probe fails once (consumed by probeErrs=1), but Sweep must
	// continue, log, and process the rest of the event without crashing.
	require.NoError(t, comp.Sweep(ctx))
}

func TestEventIDFromPendingKey(t *testing.T) {
	cases := map[string]struct {
		key string
		id  string
		ok  bool
	}{
		"valid":            {"cets:v1:resv:evt_abc:pending", "evt_abc", true},
		"wrong prefix":     {"some:other:key", "", false},
		"wrong suffix":     {"cets:v1:resv:evt_abc:hold:x", "", false},
		"empty event id":   {"cets:v1:resv::pending", "", false},
		"event with colon": {"cets:v1:resv:evt:colon:pending", "evt:colon", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			id, ok := eventIDFromPendingKey(tc.key)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.id, id)
		})
	}
}
