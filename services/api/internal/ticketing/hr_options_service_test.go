package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminHROptionsReturnsDistinctEmployeeSites(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	options, err := service.AdminHROptions(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin})

	require.NoError(t, err)
	assert.Contains(t, options.Sites, AdminOption{Value: "*", Label: "所有廠區"})
	assert.Contains(t, options.Sites, AdminOption{Value: "Taipei HQ", Label: "Taipei HQ"})
	assert.Contains(t, options.Sites, AdminOption{Value: "Tainan HQ", Label: "Tainan HQ"})
}

func TestAdminHROptionsRejectsEmployees(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.AdminHROptions(ctx, Actor{ID: "E1001", Role: RoleEmployee})

	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
}
