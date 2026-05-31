package ticketing

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBookingRequest(t *testing.T) {
	identity, err := normalizeBookingRequest(Actor{ID: "E1001", Role: RoleEmployee}, BookingRequest{IdempotencyKey: " key ", FamilyCount: 2})
	require.NoError(t, err)
	assert.Equal(t, bookingIdentity{idempotencyKey: "key", employeeID: "E1001", familyCount: 2}, identity)

	_, err = normalizeBookingRequest(Actor{ID: "E1001", Role: RoleEmployee}, BookingRequest{})
	require.ErrorContains(t, err, "idempotency_key")

	_, err = normalizeBookingRequest(Actor{ID: "E1001", Role: RoleEmployee}, BookingRequest{IdempotencyKey: "key", FamilyCount: maxFamilyCount + 1})
	require.ErrorContains(t, err, "family_count")

	_, err = normalizeBookingRequest(Actor{ID: "E1001", Role: RoleEmployee}, BookingRequest{EmployeeID: "E2002", IdempotencyKey: "key"})
	require.ErrorContains(t, err, "themselves")
}

func TestBookingPureHelpers(t *testing.T) {
	assert.Equal(t, "booking.confirmed", bookingAction(RegistrationConfirmed))
	assert.Equal(t, "booking.waitlisted", bookingAction(RegistrationWaitlisted))

	event := Event{CapacityType: CapacityTypeLimited}
	assert.Equal(t, 2, remainingForNewBooking(event, RegistrationConfirmed, 5, 2))
	assert.Zero(t, remainingForNewBooking(event, RegistrationWaitlisted, 5, 2))
	assert.Zero(t, remainingForNewBooking(Event{CapacityType: CapacityTypeUnlimited}, RegistrationConfirmed, 5, 2))
}

func TestValidateBookingEligibility(t *testing.T) {
	rule := EligibilityRule{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"}
	employee := Employee{EmployeeID: "E1001", Department: "Engineering", Site: "Taipei", JobGrade: 6, EmploymentStatus: "active"}
	require.NoError(t, validateBookingEligibility(Actor{ID: "E1001"}, employee, rule))

	err := validateBookingEligibility(Actor{ID: "E1001"}, Employee{EmployeeID: "E1001", Department: "Finance", Site: "Taipei", JobGrade: 6, EmploymentStatus: "active"}, rule)
	require.Error(t, err)

	err = validateBookingEligibility(Actor{ID: "E1001", Claims: &ProviderClaims{}}, employee, rule)
	require.ErrorContains(t, err, ErrMissingClaims.Error())
}

func TestValidateRetryableNotificationDelivery(t *testing.T) {
	require.NoError(t, validateRetryableNotificationDelivery(NotificationDelivery{Channel: "email", Status: deliveryStatusFailed, OutboxID: "out_1"}))
	require.NoError(t, validateRetryableNotificationDelivery(NotificationDelivery{Channel: "email", Status: deliveryStatusDeadLetter, OutboxID: "out_1"}))

	require.ErrorContains(t, validateRetryableNotificationDelivery(NotificationDelivery{Channel: "in_app", Status: deliveryStatusFailed, OutboxID: "out_1"}), "only email")
	require.ErrorContains(t, validateRetryableNotificationDelivery(NotificationDelivery{Channel: "email", Status: deliveryStatusSent, OutboxID: "out_1"}), "only failed")
	require.ErrorContains(t, validateRetryableNotificationDelivery(NotificationDelivery{Channel: "email", Status: deliveryStatusFailed}), "outbox")
}

func TestNotificationRetryAndCursorHelpers(t *testing.T) {
	assert.Empty(t, notificationEmployeeRef(" "))
	assert.Equal(t, "E100****", notificationEmployeeRef("E1001"))

	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(now.Format(time.RFC3339Nano) + "|del_1"))
	gotTime, gotID := parseNotificationDeliveryCursor(encoded)
	assert.True(t, gotTime.Equal(now))
	assert.Equal(t, "del_1", gotID)

	_, _, err := ParseNotificationDeliveryCursorStrict("invalid")
	require.ErrorContains(t, err, "invalid")
}

func TestIdempotencyAndEmailDeliveryHelpers(t *testing.T) {
	completed := bookingIdempotencyResult{
		EventID:        "evt_1",
		EmployeeID:     "E1001",
		FamilyCount:    1,
		RegistrationID: "reg_1",
		CompletedAt:    time.Now(),
	}
	require.NoError(t, validateBookingIdempotencyResult(completed, "evt_1", "E1001", 1))
	require.ErrorContains(t, validateBookingIdempotencyResult(completed, "evt_2", "E1001", 1), "different booking")
	completed.RegistrationID = ""
	require.ErrorContains(t, validateBookingIdempotencyResult(completed, "evt_1", "E1001", 1), "not ready")

	policy := OutboxRetryPolicy{MaxAttempts: 3}
	status, lastErr := emailDeliveryResult(nil, "E1001", outboxClaim{attempts: 1}, policy)
	assert.Equal(t, deliveryStatusSent, status)
	assert.Empty(t, lastErr)

	status, lastErr = emailDeliveryResult(errors.New("smtp failed for E1001"), "E1001", outboxClaim{attempts: 3}, policy)
	assert.Equal(t, deliveryStatusDeadLetter, status)
	assert.NotContains(t, lastErr, "E1001")
}

func TestPayloadHelpers(t *testing.T) {
	claim := outboxClaim{eventType: "notification.requested.v2", schemaVersion: 2}
	body, ok := decodeOutboxPayload(`{"schema_version":2,"event_type":"notification.requested.v2","occurred_at":"2026-05-28T10:00:00Z","payload":{"employee_id":"E1001"}}`, claim)
	require.True(t, ok)
	assert.Equal(t, "E1001", stringFromPayload(body, "employee_id"))

	_, ok = decodeOutboxPayload(`{"schema_version":3,"payload":{}}`, claim)
	require.False(t, ok)

	assert.True(t, validOutboxV2OccurredAt("2026-05-28T10:00:00Z"))
	assert.False(t, validOutboxV2OccurredAt("2026-05-28T10:00:00+08:00"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.Equal(t, context.Canceled, contextError(ctx, errors.New("network error")))
}
