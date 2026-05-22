package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListTicketsReturnsOwningEmployeeTickets(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Employee Ticket List",
		Location: "Taipei HQ",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "list-ticket-1",
	})
	require.NoError(t, err)
	require.NotNil(t, booking.Ticket)

	tickets, err := service.ListTickets(ctx, Actor{ID: "E1001", Role: RoleEmployee}, "")
	require.NoError(t, err)
	require.Len(t, tickets, 1)

	got := tickets[0]
	assert.Equal(t, booking.Ticket.TicketID, got.TicketID)
	assert.Equal(t, event.EventID, got.EventID)
	assert.Equal(t, "E1001", got.EmployeeID)
	assert.Equal(t, "Employee Ticket List", got.EventTitle)
	assert.Equal(t, "Ariel Chen", got.EmployeeName)
	assert.True(t, got.NonTransferable)
	assert.NotEmpty(t, got.SignedToken)
	assert.NotEmpty(t, got.QRPayload)
}

func TestListTicketsRejectsNonOwnerAndNonEmployeeActors(t *testing.T) {
	service := &Service{}

	_, err := service.ListTickets(context.Background(), Actor{ID: "E1002", Role: RoleEmployee}, "E1001")
	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))

	_, err = service.ListTickets(context.Background(), Actor{ID: "admin-1", Role: RoleActivityAdmin}, "E1001")
	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))

	_, err = service.ListTickets(context.Background(), Actor{}, "E1001")
	require.Error(t, err)
	assert.Equal(t, 401, ErrorStatus(err))
}
