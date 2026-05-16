package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"event-ticket-system/internal/ticketing"
)

type ProviderAuthConfig struct {
	Secret string
}

type ProviderVerifier struct {
	secret []byte
	now    func() time.Time
}

type providerClaims struct {
	EmployeeID       string   `json:"employee_id"`
	DisplayName      string   `json:"display_name"`
	JobTitle         *string  `json:"job_title,omitempty"`
	RoleClaims       []string `json:"role_claims"`
	Department       string   `json:"department"`
	Site             string   `json:"site"`
	City             string   `json:"city"`
	Grade            int      `json:"grade"`
	EmploymentStatus string   `json:"employment_status"`
	ExpiresAt        int64    `json:"exp"`
}

var errProviderRoleRejected = errors.New("provider role is not allowed")

func NewProviderVerifier(config ProviderAuthConfig) *ProviderVerifier {
	return &ProviderVerifier{
		secret: []byte(strings.TrimSpace(config.Secret)),
		now:    time.Now,
	}
}

func (v *ProviderVerifier) IdentityFromToken(token string) (authIdentity, error) {
	var identity authIdentity
	token = strings.TrimSpace(token)
	if token == "" {
		return identity, errors.New("provider token is required")
	}
	if len(v.secret) == 0 {
		return identity, errors.New("provider verifier is not configured")
	}

	claims, err := v.Verify(token)
	if err != nil {
		return identity, err
	}
	mappedRoles := mappedProviderRoles(claims.RoleClaims)
	if len(mappedRoles) != 1 {
		return identity, errProviderRoleRejected
	}
	return authIdentity{
		Actor: ticketing.Actor{
			ID:   claims.EmployeeID,
			Role: mappedRoles[0],
			Claims: &ticketing.ProviderClaims{
				Department:       claims.Department,
				Site:             claims.Site,
				City:             claims.City,
				Grade:            claims.Grade,
				EmploymentStatus: claims.EmploymentStatus,
			},
		},
		Claims:    providerClaimsPayload(claims, mappedRoles),
		ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
		Source:    authSourceProvider,
	}, nil
}

func (v *ProviderVerifier) Verify(token string) (providerClaims, error) {
	var claims providerClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, errors.New("invalid provider token format")
	}
	expected := v.signature(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return claims, errors.New("invalid provider token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, err
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, err
	}
	claims = normalizeProviderClaims(claims)
	if err := validateProviderClaims(claims, v.now().UTC()); err != nil {
		return claims, err
	}
	return claims, nil
}

func (v *ProviderVerifier) Sign(claims providerClaims) (string, error) {
	if len(v.secret) == 0 {
		return "", errors.New("provider verifier is not configured")
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	return payloadPart + "." + v.signature(payloadPart), nil
}

func (v *ProviderVerifier) signature(payloadPart string) string {
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func normalizeProviderClaims(claims providerClaims) providerClaims {
	claims.EmployeeID = strings.TrimSpace(claims.EmployeeID)
	claims.DisplayName = strings.TrimSpace(claims.DisplayName)
	claims.Department = strings.TrimSpace(claims.Department)
	claims.Site = strings.TrimSpace(claims.Site)
	claims.City = strings.TrimSpace(claims.City)
	claims.EmploymentStatus = strings.TrimSpace(claims.EmploymentStatus)
	claims.RoleClaims = trimStringSlice(claims.RoleClaims)
	if claims.JobTitle != nil {
		jobTitle := strings.TrimSpace(*claims.JobTitle)
		if jobTitle == "" {
			claims.JobTitle = nil
		} else {
			claims.JobTitle = &jobTitle
		}
	}
	return claims
}

func validateProviderClaims(claims providerClaims, now time.Time) error {
	if claims.EmployeeID == "" ||
		claims.DisplayName == "" ||
		len(claims.RoleClaims) == 0 ||
		claims.Department == "" ||
		claims.Site == "" ||
		claims.City == "" ||
		claims.Grade <= 0 ||
		claims.EmploymentStatus == "" ||
		claims.ExpiresAt <= 0 {
		return errors.New("provider claims are incomplete")
	}
	if now.Unix() >= claims.ExpiresAt {
		return errors.New("provider token expired")
	}
	return nil
}

func mappedProviderRoles(roleClaims []string) []string {
	allowed := map[string]string{
		ticketing.RoleEmployee:      ticketing.RoleEmployee,
		ticketing.RoleActivityAdmin: ticketing.RoleActivityAdmin,
		ticketing.RoleCheckinStaff:  ticketing.RoleCheckinStaff,
		ticketing.RoleHRAdmin:       ticketing.RoleHRAdmin,
		ticketing.RoleSystemAdmin:   ticketing.RoleSystemAdmin,
	}
	seen := map[string]bool{}
	var mapped []string
	for _, roleClaim := range roleClaims {
		role, ok := allowed[strings.TrimSpace(roleClaim)]
		if !ok || seen[role] {
			continue
		}
		seen[role] = true
		mapped = append(mapped, role)
	}
	return mapped
}

func trimStringSlice(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}
