package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemainingForNewBookingCoversConfirmedAndWaitlistedBranches(t *testing.T) {
	limited := Event{CapacityType: CapacityTypeLimited}
	unlimited := Event{CapacityType: CapacityTypeUnlimited}

	assert.Equal(t, 2, remainingForNewBooking(limited, RegistrationConfirmed, 3, 0))
	assert.Equal(t, 0, remainingForNewBooking(limited, RegistrationWaitlisted, 3, 3))
	assert.Equal(t, 0, remainingForNewBooking(unlimited, RegistrationConfirmed, 0, 0))
}

func TestPrepareCreateEventInputValidationAndDefaults(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	service := NewService(nil, NewSigner("test-secret"), nil)
	service.now = func() time.Time { return now }

	tests := []struct {
		name string
		req  CreateEventRequest
		want string
	}{
		{
			name: "title required",
			req:  CreateEventRequest{},
			want: "title is required",
		},
		{
			name: "status must be supported",
			req:  CreateEventRequest{Title: "Invalid Status", Capacity: 1, Status: "archived"},
			want: "status must be draft or published",
		},
		{
			name: "registration window must be ordered",
			req: CreateEventRequest{
				Title:             "Bad Window",
				Capacity:          1,
				RegistrationStart: now,
				RegistrationClose: now,
			},
			want: "registration_start must be before registration_close",
		},
		{
			name: "event end must follow start",
			req: CreateEventRequest{
				Title:             "Bad Event Window",
				Capacity:          1,
				StartsAt:          now.Add(3 * time.Hour),
				EndsAt:            now.Add(3 * time.Hour),
				RegistrationStart: now,
				RegistrationClose: now.Add(time.Hour),
			},
			want: "starts_at must be before ends_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.prepareCreateEventInput(tt.req)

			require.ErrorContains(t, err, tt.want)
		})
	}

	input, err := service.prepareCreateEventInput(CreateEventRequest{
		Title:       "  Defaulted Event  ",
		Description: "phase3 defaults",
		Location:    "Taipei HQ",
		Capacity:    5,
		Rule: RuleInput{
			Department:       "Engineering",
			Site:             "Taipei HQ",
			MinGrade:         5,
			EmploymentStatus: "active",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "Defaulted Event", input.event.Title)
	assert.Equal(t, EventStatusPublished, input.event.Status)
	assert.Equal(t, "qr", input.event.EntryMethod)
	assert.Equal(t, "eligible", input.event.Visibility)
	assert.Equal(t, AllocationModeFCFS, input.event.AllocationMode)
	assert.Equal(t, "Taipei", input.event.EventCity)
	assert.Equal(t, "Taipei HQ", input.event.EventSite)
	require.NotNil(t, input.event.Capacity)
	assert.Equal(t, 5, *input.event.Capacity)
	assert.Equal(t, now.Add(7*24*time.Hour), input.event.StartsAt)
	assert.Equal(t, input.event.StartsAt.Add(24*time.Hour), input.event.EndsAt)
	assert.Equal(t, now.Add(-time.Hour), input.event.RegistrationStart)
	assert.Equal(t, input.event.StartsAt.Add(-time.Hour), input.event.RegistrationClose)
}

func TestEventSummaryCompatibilityHelpers(t *testing.T) {
	assert.Nil(t, eligibilityReasons(""))
	assert.Equal(t, []string{"department mismatch"}, eligibilityReasons("department mismatch"))

	withReason := EventSummary{
		Eligibility: EligibilityDecision{
			Eligible: false,
			Reasons:  []string{"site mismatch"},
		},
	}
	applyEligibilityCompatibilityFields(&withReason)
	assert.False(t, withReason.Eligible)
	assert.Equal(t, "site mismatch", withReason.EligibilityReason)

	withoutReason := EventSummary{
		Eligibility: EligibilityDecision{Eligible: true},
	}
	applyEligibilityCompatibilityFields(&withoutReason)
	assert.True(t, withoutReason.Eligible)
	assert.Empty(t, withoutReason.EligibilityReason)
}
