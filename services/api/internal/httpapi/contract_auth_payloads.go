package httpapi

const claimsStatusComplete = "complete"

type employeeClaimsPayload struct {
	EmployeeID   string   `json:"employee_id"`
	DisplayName  string   `json:"display_name"`
	JobTitle     *string  `json:"job_title"`
	RoleClaims   []string `json:"role_claims"`
	MappedRoles  []string `json:"mapped_roles"`
	Department   string   `json:"department"`
	Site         string   `json:"site"`
	City         string   `json:"city"`
	ClaimsStatus string   `json:"claims_status"`
}

func authClaimsResponse(identity authIdentity) employeeClaimsPayload {
	return identity.Claims
}

func providerClaimsPayload(claims providerClaims, mappedRoles []string) employeeClaimsPayload {
	return employeeClaimsPayload{
		EmployeeID:   claims.EmployeeID,
		DisplayName:  claims.DisplayName,
		JobTitle:     claims.JobTitle,
		RoleClaims:   append([]string(nil), claims.RoleClaims...),
		MappedRoles:  append([]string(nil), mappedRoles...),
		Department:   claims.Department,
		Site:         claims.Site,
		City:         claims.City,
		ClaimsStatus: claimsStatusComplete,
	}
}
