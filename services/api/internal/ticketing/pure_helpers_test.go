package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeOutboxPayloadValidatesEnvelopeWithoutExternalServices(t *testing.T) {
	v1Payload, ok := decodeOutboxPayload(`{"employee_id":" E1001 "}`, outboxClaim{schemaVersion: 1})
	require.True(t, ok)
	assert.Equal(t, " E1001 ", v1Payload["employee_id"])

	invalidPayload, ok := decodeOutboxPayload(`["not-an-object"]`, outboxClaim{schemaVersion: 1})
	assert.False(t, ok)
	assert.Nil(t, invalidPayload)

	claim := outboxClaim{
		outboxID:       "out-v2",
		eventType:      "notification.requested.v2",
		schemaVersion:  2,
		idempotencyKey: "notification.requested:out-v2",
		partitionKey:   "E1001",
	}
	v2Payload, ok := decodeOutboxPayload(`{
		"event_id":"out-v2",
		"event_type":"notification.requested.v2",
		"schema_version":2,
		"occurred_at":"2026-05-31T08:00:00.123Z",
		"idempotency_key":"notification.requested:out-v2",
		"partition_key":"E1001",
		"payload":{"recipient_employee_id":"E1001","channel":"email"}
	}`, claim)
	require.True(t, ok)
	assert.Equal(t, "E1001", v2Payload["recipient_employee_id"])

	mismatchedEnvelope, ok := decodeOutboxPayload(`{
		"event_id":"out-v2",
		"event_type":"notification.requested.v2",
		"schema_version":1,
		"occurred_at":"2026-05-31T08:00:00Z",
		"idempotency_key":"notification.requested:out-v2",
		"partition_key":"E1001",
		"payload":{"recipient_employee_id":"E1001"}
	}`, claim)
	assert.False(t, ok)
	assert.Nil(t, mismatchedEnvelope)
	assert.True(t, validOutboxV2OccurredAt("2026-05-31T08:00:00.123Z"))
	assert.False(t, validOutboxV2OccurredAt("2026-05-31T16:00:00+08:00"))
}

func TestNotificationPayloadUsesRecipientAndChannelRules(t *testing.T) {
	payload := map[string]interface{}{
		"employee_id":            "E1001",
		"recipient_employee_id":  "E2002",
		"channel":                "in_app",
		"unused_sensitive_field": "ignored",
	}

	assert.Equal(t, "E2002", recipientEmployeeIDForOutbox("notification.requested.v2", payload))
	assert.Equal(t, "E1001", recipientEmployeeIDForOutbox("booking.confirmed", payload))

	channels := deliveryChannelsForOutbox("booking.confirmed", payload, true)
	assert.True(t, channels.valid)
	assert.True(t, channels.inApp)
	assert.True(t, channels.email)

	channels = deliveryChannelsForOutbox("notification.requested.v2", payload, true)
	assert.True(t, channels.valid)
	assert.True(t, channels.inApp)
	assert.False(t, channels.email)

	payload["channel"] = "email"
	channels = deliveryChannelsForOutbox("notification.requested.v2", payload, false)
	assert.True(t, channels.valid)
	assert.False(t, channels.inApp)
	assert.False(t, channels.email)

	payload["channel"] = "sms"
	channels = deliveryChannelsForOutbox("notification.requested.v2", payload, true)
	assert.False(t, channels.valid)
	assert.False(t, channels.inApp)
	assert.False(t, channels.email)
}

func TestBookingNotificationPayloadAddsCrossCityWarningOnlyForConfirmedBookings(t *testing.T) {
	event := Event{
		EventID:      "evt-1",
		CapacityType: CapacityTypeLimited,
		EventCity:    "Taipei HQ",
	}
	actor := Actor{Claims: &ProviderClaims{City: "Hsinchu Science Park"}}
	registration := Registration{
		RegistrationID: "reg-1",
		EventID:        "evt-1",
		EmployeeID:     "E1001",
		Status:         RegistrationConfirmed,
		FamilyCount:    0,
	}

	payload := bookingNotificationPayload(registration, event, actor)
	assert.Equal(t, string(WarningCrossCity), payload["warning_code"])
	assert.Equal(t, "Taipei", payload["event_city"])

	registration.Status = RegistrationReceived
	payload = bookingNotificationPayload(registration, event, actor)
	assert.NotContains(t, payload, "warning_code")

	registration.Status = RegistrationConfirmed
	payload = bookingNotificationPayload(registration, event, Actor{})
	assert.NotContains(t, payload, "warning_code")
}

func TestMetadataHelpersKeepAuditContextAndSkipBlankOverrides(t *testing.T) {
	startsAt := time.Date(2026, 5, 31, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	metadata := eventContextMetadata(Event{
		EventID:   "evt-1",
		Title:     "  Demo Event  ",
		EventCity: " Taipei ",
		EventSite: "",
		StartsAt:  startsAt,
	})

	assert.Equal(t, "evt-1", metadata["event_id"])
	assert.Equal(t, "Demo Event", metadata["event_title"])
	assert.Equal(t, "Taipei", metadata["event_city"])
	assert.NotContains(t, metadata, "event_site")
	assert.Equal(t, "2026-05-31T00:00:00Z", metadata["starts_at"])

	merged := mergeMetadata(metadata, map[string]interface{}{
		"event_title": " ",
		"actor_id":    "admin-1",
		"event_city":  "Hsinchu",
	})
	assert.Equal(t, "Demo Event", merged["event_title"])
	assert.Equal(t, "admin-1", merged["actor_id"])
	assert.Equal(t, "Hsinchu", merged["event_city"])

	payload := ticketOutboxPayload(Ticket{
		TicketID:   "ticket-1",
		EventID:    "evt-1",
		EmployeeID: "E1001",
	}, map[string]interface{}{"reason": "issued"})
	assert.Equal(t, "ticket-1", payload["ticket_id"])
	assert.Equal(t, "E1001", payload["employee_id"])
	assert.Equal(t, "issued", payload["reason"])
}

func TestTicketHolderFromTicketNormalizesDisplayFields(t *testing.T) {
	holder := ticketHolderFromTicket(Ticket{
		EmployeeName: "Ariel Chen",
		Department:   "Engineering",
		City:         " Taipei HQ ",
	})

	assert.Equal(t, TicketHolder{
		DisplayName: "Ariel Chen",
		Department:  "Engineering",
		City:        "Taipei",
	}, holder)
}

func TestWorkerSchemaAndTelemetryHelpersRejectUnsafeValues(t *testing.T) {
	assert.True(t, supportedOutboxSchemaVersion(1))
	assert.True(t, supportedOutboxSchemaVersion(2))
	assert.False(t, supportedOutboxSchemaVersion(99))
	assert.Contains(t, unsupportedOutboxSchemaVersionError(99), "unknown_schema_version")

	assert.True(t, supportedOutboxEventType("booking.confirmed"))
	assert.False(t, supportedOutboxEventType("unknown.event"))
	unsafeType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJF1001xyz"
	assert.False(t, supportedOutboxEventType(unsafeType))
	assert.Equal(t, outboxWorkerKindUnknown, safeOutboxTelemetryEventType(unsafeType))
	assert.Equal(t, outboxWorkerKindUnknown, safeOutboxTelemetryWorkerKind(unsafeType))
	formattedError := unsupportedOutboxEventTypeError(unsafeType)
	assert.Contains(t, formattedError, "event_type=unknown")
	assert.NotContains(t, formattedError, unsafeType)
	assert.NotContains(t, formattedError, "e1001@cets.local")
	assert.NotContains(t, formattedError, "eyJhbGci")
}

func TestWorkerKindFilterAndRetryPolicyStayDeterministic(t *testing.T) {
	assert.Equal(t, []string{"notification", "export", "projection"}, outboxWorkerKindFilter([]string{
		" Notification ",
		"export",
		"",
		"notification",
		"PROJECTION",
	}))
	assert.True(t, retryableNotificationOutboxEventType("booking.confirmed"))
	assert.False(t, retryableNotificationOutboxEventType(outboxEventReportExportRequestedV2))

	policy := normalizeOutboxRetryPolicy(OutboxRetryPolicy{
		MaxAttempts: -1,
		BackoffBase: 0,
		BackoffMax:  time.Second,
	})
	assert.Equal(t, 0, policy.MaxAttempts)
	assert.Equal(t, time.Minute, policy.BackoffBase)
	assert.Equal(t, time.Minute, policy.BackoffMax)
	assert.True(t, policy.exhausted(0))

	policy = OutboxRetryPolicy{MaxAttempts: 3, BackoffBase: time.Second, BackoffMax: 5 * time.Second}
	assert.False(t, policy.exhausted(2))
	assert.True(t, policy.exhausted(3))
	firstBackoff := policy.backoffForOutbox("out-1", 1)
	secondBackoff := policy.backoffForOutbox("out-1", 2)
	cappedBackoff := policy.backoffForOutbox("out-1", 10)
	assert.GreaterOrEqual(t, firstBackoff, time.Second)
	assert.Less(t, firstBackoff, 1250*time.Millisecond)
	assert.GreaterOrEqual(t, secondBackoff, 2*time.Second)
	assert.Less(t, secondBackoff, 2500*time.Millisecond)
	assert.LessOrEqual(t, cappedBackoff, 5*time.Second)
	assert.Equal(t, secondBackoff, policy.backoffForOutbox("out-1", 2))
}

func TestEventCapacityRulesStayPureAndExplicit(t *testing.T) {
	capacityType, capacity, allowsFamily, err := normalizeEventCapacity("", 4, false)
	require.NoError(t, err)
	assert.Equal(t, CapacityTypeLimited, capacityType)
	require.NotNil(t, capacity)
	assert.Equal(t, 4, *capacity)
	assert.False(t, allowsFamily)

	capacityType, capacity, allowsFamily, err = normalizeEventCapacity(CapacityTypeUnlimited, 0, false)
	require.NoError(t, err)
	assert.Equal(t, CapacityTypeUnlimited, capacityType)
	assert.Nil(t, capacity)
	assert.True(t, allowsFamily)

	_, _, _, err = normalizeEventCapacity(CapacityTypeLimited, 1, true)
	assert.ErrorContains(t, err, "limited events cannot allow family attendees")
	_, _, _, err = normalizeEventCapacity(CapacityTypeUnlimited, 1, false)
	assert.ErrorContains(t, err, "capacity must be null for unlimited events")
	assert.ErrorContains(t, validateAllocationModeForCapacity(AllocationModeLottery, CapacityTypeUnlimited), "lottery allocation requires limited capacity")

	limited, err := limitedCapacity(Event{CapacityType: CapacityTypeLimited, Capacity: intPtr(7)})
	require.NoError(t, err)
	assert.Equal(t, 7, limited)
	_, err = limitedCapacity(Event{CapacityType: CapacityTypeUnlimited})
	assert.ErrorContains(t, err, "unlimited event booking allocation is not implemented")
}

func TestContextErrorPrefersCancellationCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deadlineCtx, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer deadlineCancel()

	assert.ErrorIs(t, contextError(ctx, assert.AnError), context.Canceled)
	assert.ErrorIs(t, contextError(deadlineCtx, assert.AnError), context.DeadlineExceeded)
	assert.ErrorIs(t, contextError(context.Background(), assert.AnError), assert.AnError)
	assert.NoError(t, contextError(context.Background(), nil))
}
