package ticketing

import "strings"

const (
	errMissingActorHeaders = "missing actor headers"
	errRoleNotAllowed      = "role is not allowed"
)

func authorizeEmployeeRead(actor Actor, employeeID string) (string, error) {
	if actor.ID == "" || actor.Role == "" {
		return "", unauthorized(errMissingActorHeaders)
	}
	employeeID = strings.TrimSpace(employeeID)
	switch actor.Role {
	case RoleEmployee:
		if employeeID == "" {
			return actor.ID, nil
		}
		if employeeID != actor.ID {
			return "", forbidden("employees may only view their own data")
		}
		return employeeID, nil
	case RoleActivityAdmin, RoleHRAdmin, RoleSystemAdmin:
		return employeeID, nil
	default:
		return "", forbidden(errRoleNotAllowed)
	}
}

func requireRole(actor Actor, role string) error {
	if actor.ID == "" || actor.Role == "" {
		return unauthorized(errMissingActorHeaders)
	}
	if !roleMatches(actor.Role, role) {
		return forbidden(errRoleNotAllowed)
	}
	return nil
}

func requireAnyRole(actor Actor, roles ...string) error {
	if actor.ID == "" || actor.Role == "" {
		return unauthorized(errMissingActorHeaders)
	}
	for _, role := range roles {
		if roleMatches(actor.Role, role) {
			return nil
		}
	}
	return forbidden(errRoleNotAllowed)
}

func roleMatches(actual string, required string) bool {
	if actual == required {
		return true
	}
	return actual == RoleSystemAdmin && required == RoleHRAdmin
}
