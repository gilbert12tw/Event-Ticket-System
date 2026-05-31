package ticketing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeEmployeeReadRejectsMissingActor(t *testing.T) {
	_, err := authorizeEmployeeRead(Actor{}, "E1001")

	require.Error(t, err)
	assert.Equal(t, 401, ErrorStatus(err))
	assert.Equal(t, errMissingActorHeaders, ErrorMessage(err))
}

func TestAuthorizeEmployeeReadRejectsUnsupportedRole(t *testing.T) {
	_, err := authorizeEmployeeRead(Actor{ID: "contractor-1", Role: "contractor"}, "E1001")

	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
	assert.Equal(t, errRoleNotAllowed, ErrorMessage(err))
}

func TestRequireAnyRoleRejectsMissingActor(t *testing.T) {
	err := requireAnyRole(Actor{}, RoleActivityAdmin, RoleSystemAdmin)

	require.Error(t, err)
	assert.Equal(t, 401, ErrorStatus(err))
	assert.Equal(t, errMissingActorHeaders, ErrorMessage(err))
}
