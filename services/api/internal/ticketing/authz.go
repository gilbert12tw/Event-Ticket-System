package ticketing

import "strings"

func authorizeEmployeeRead(actor Actor, employeeID string) (string, error) {
	if actor.ID == "" || actor.Role == "" {
		return "", unauthorized("missing actor headers")
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
		return "", forbidden("role is not allowed")
	}
}

func requireRole(actor Actor, role string) error {
	if actor.ID == "" || actor.Role == "" {
		return unauthorized("missing actor headers")
	}
	if !roleMatches(actor.Role, role) {
		return forbidden("role is not allowed")
	}
	return nil
}

func requireAnyRole(actor Actor, roles ...string) error {
	if actor.ID == "" || actor.Role == "" {
		return unauthorized("missing actor headers")
	}
	for _, role := range roles {
		if roleMatches(actor.Role, role) {
			return nil
		}
	}
	return forbidden("role is not allowed")
}

func roleMatches(actual string, required string) bool {
	if actual == required {
		return true
	}
	return actual == RoleSystemAdmin && required == RoleHRAdmin
}
