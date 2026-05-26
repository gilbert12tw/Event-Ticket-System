package ticketing

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookingOutboxPayloadIncludesOnlyRedactedCrossCityContext(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:     "Cross City Notification",
		Location:  "Taipei HQ",
		EventCity: "Taipei",
		Capacity:  2,
		Status:    EventStatusPublished,
		Rule:      RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee, Claims: &ProviderClaims{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Hsinchu",
		Grade:            6,
		EmploymentStatus: "active",
	}}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "cross-city-notification"})
	require.NoError(t, err)

	payloadText := workerOutboxPayload(t, service, ctx, booking.Registration.RegistrationID, "booking.confirmed")
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payloadText), &payload))
	assert.Equal(t, string(WarningCrossCity), payload["warning_code"])
	assert.Equal(t, "Taipei", payload["event_city"])
	assert.NotContains(t, payload, "employee_city")
	assert.NotContains(t, payloadText, "Hsinchu")
	assert.NotContains(t, payloadText, "Ariel Chen")
	assert.NotContains(t, payloadText, "@")
	assert.NotContains(t, payloadText, "signed_token")
	assert.NotContains(t, payloadText, "qr_payload")

	sender := &recordingNotificationSender{}
	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, sender.messages, 1)
	assert.Contains(t, sender.messages[0].Body, "This activity is in Taipei")
	assert.NotContains(t, sender.messages[0].Body, "Hsinchu")
}

func TestBookingOutboxPayloadOmitsCrossCityContextForSameCity(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:     "Same City Notification",
		Location:  "Taipei HQ",
		EventCity: "Taipei",
		Capacity:  2,
		Status:    EventStatusPublished,
		Rule:      RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee, Claims: &ProviderClaims{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            6,
		EmploymentStatus: "active",
	}}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "same-city-notification"})
	require.NoError(t, err)

	payloadText := workerOutboxPayload(t, service, ctx, booking.Registration.RegistrationID, "booking.confirmed")
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payloadText), &payload))
	assert.NotContains(t, payload, "warning_code")
	assert.NotContains(t, payload, "event_city")

	sender := &recordingNotificationSender{}
	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, sender.messages, 1)
	assert.NotContains(t, sender.messages[0].Body, "This activity is in")
}

func TestBookingOutboxPayloadOmitsCrossCityContextForWaitlistedBooking(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:     "Waitlist Cross City Notification",
		Location:  "Taipei HQ",
		EventCity: "Taipei",
		Capacity:  1,
		Status:    EventStatusPublished,
		Rule:      RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee, Claims: &ProviderClaims{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Hsinchu",
		Grade:            6,
		EmploymentStatus: "active",
	}}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "waitlist-cross-city-confirmed"})
	require.NoError(t, err)
	waitlisted, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee, Claims: &ProviderClaims{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Hsinchu",
		Grade:            5,
		EmploymentStatus: "active",
	}}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "waitlist-cross-city-waitlisted"})
	require.NoError(t, err)
	require.Equal(t, RegistrationWaitlisted, waitlisted.Registration.Status)

	payloadText := workerOutboxPayload(t, service, ctx, waitlisted.Registration.RegistrationID, "booking.waitlisted")
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payloadText), &payload))
	assert.NotContains(t, payload, "warning_code")
	assert.NotContains(t, payload, "event_city")
	assert.NotContains(t, payloadText, "Hsinchu")
}
