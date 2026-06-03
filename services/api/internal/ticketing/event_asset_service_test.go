package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveEventPosterReplacesPosterAndAudits(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Poster Event",
		Capacity: 4,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	actor := Actor{ID: "admin-1", Role: RoleActivityAdmin}

	first, err := service.SaveEventPoster(ctx, actor, event.EventID, EventAssetInput{
		ObjectKey:   "events/" + event.EventID + "/poster.png",
		FileName:    "poster.png",
		ContentType: "image/png",
		SizeBytes:   12,
	})
	require.NoError(t, err)
	second, err := service.SaveEventPoster(ctx, actor, event.EventID, EventAssetInput{
		ObjectKey:   "events/" + event.EventID + "/poster.webp",
		FileName:    "poster.webp",
		ContentType: "image/webp",
		SizeBytes:   20,
	})
	require.NoError(t, err)

	poster, err := service.GetEventPoster(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID)
	require.NoError(t, err)
	assert.NotEqual(t, first.AssetID, second.AssetID)
	assert.Equal(t, second.AssetID, poster.AssetID)
	assert.Equal(t, "image/webp", poster.ContentType)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_assets WHERE event_id = $1`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'event.poster.updated' AND entity_id = $1`, event.EventID, 2)
}

func TestGetEventPosterReturnsNotFoundWhenMissing(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "No Poster Event",
		Capacity: 4,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.GetEventPoster(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID)

	require.Error(t, err)
	assert.Equal(t, 404, ErrorStatus(err))
}
