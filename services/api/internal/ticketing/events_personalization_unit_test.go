package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApplyEventListPersonalizationHandlesMissingClaimsAndRegistration(t *testing.T) {
	summary := EventSummary{Event: Event{EventID: "evt-1", CapacityType: CapacityTypeLimited}}
	expiresAt := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	personalization := eventListPersonalization{
		MissingClaims: true,
		RegistrationsByEvent: map[string]Registration{
			"evt-1": {RegistrationID: "reg-1", EventID: "evt-1", EmployeeID: "E1001", Status: RegistrationConfirmed},
		},
		TicketsByRegistration: map[string]*Ticket{
			"reg-1": {
				TicketID:        "tic-1",
				RegistrationID:  "reg-1",
				EventID:         "evt-1",
				EmployeeID:      "E1001",
				Status:          "active",
				SignedToken:     "secret-token",
				QRPayload:       "secret-qr",
				ExpiresAt:       expiresAt,
				NonTransferable: true,
			},
		},
	}

	applyEventListPersonalization(&summary, personalization)

	assert.Equal(t, []string{ErrMissingClaims.Error()}, summary.Eligibility.Reasons)
	assert.False(t, summary.Eligibility.Eligible)
	assert.Equal(t, RegistrationConfirmed, summary.CurrentUserStatus)
	assert.Equal(t, "reg-1", summary.CurrentUserRegistrationID)
	if assert.NotNil(t, summary.CurrentUserTicket) {
		assert.Equal(t, "tic-1", summary.CurrentUserTicket.TicketID)
		assert.Empty(t, summary.CurrentUserTicket.SignedToken)
		assert.Empty(t, summary.CurrentUserTicket.QRPayload)
		assert.True(t, summary.CurrentUserTicket.NonTransferable)
	}
}

func TestApplyEventListPersonalizationAddsCrossCityWarningAndCooldown(t *testing.T) {
	until := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	summary := EventSummary{
		Event: Event{
			EventID:      "evt-1",
			EventCity:    "Hsinchu",
			CapacityType: CapacityTypeLimited,
		},
		Rule: EligibilityRule{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	}
	personalization := eventListPersonalization{
		EmployeeFound: true,
		Employee: Employee{
			EmployeeID:       "E1001",
			Department:       "Engineering",
			Site:             "Taipei",
			JobGrade:         6,
			EmploymentStatus: "active",
		},
		EmployeeCity: "Taipei",
		Cooldown: NoShowCooldown{
			Active:    true,
			AppliesTo: CapacityTypeLimited,
			Until:     &until,
			Reason:    "no_show_cooldown",
		},
		RegistrationsByEvent:  map[string]Registration{},
		TicketsByRegistration: map[string]*Ticket{},
	}

	applyEventListPersonalization(&summary, personalization)

	assert.True(t, summary.Eligibility.Eligible)
	assert.False(t, summary.Eligibility.CanBook)
	assert.Equal(t, summary.NoShowCooldown, summary.Eligibility.NoShowCooldown)
	assert.Equal(t, WarningCrossCity, summary.Eligibility.Warnings[0].Code)
	assert.Equal(t, "Taipei", summary.Eligibility.Warnings[0].EmployeeCity)
	assert.Equal(t, "Hsinchu", summary.Eligibility.Warnings[0].EventCity)
	assert.True(t, summary.Eligible)
	assert.Empty(t, summary.EligibilityReason)
}

func TestApplyEventListPersonalizationHandlesMissingEmployee(t *testing.T) {
	summary := EventSummary{Event: Event{EventID: "evt-404", CapacityType: CapacityTypeUnlimited}}

	applyEventListPersonalization(&summary, eventListPersonalization{
		RegistrationsByEvent:  map[string]Registration{},
		TicketsByRegistration: map[string]*Ticket{},
	})

	assert.False(t, summary.Eligibility.Eligible)
	assert.False(t, summary.Eligibility.CanBook)
	assert.Equal(t, []string{"employee not found"}, summary.Eligibility.Reasons)
	assert.Equal(t, "employee not found", summary.EligibilityReason)
}
