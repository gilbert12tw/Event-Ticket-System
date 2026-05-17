package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"event-ticket-system/internal/ticketing"
)

const authSourceProvider = "provider"

const mockProviderTokenTTL = 8 * time.Hour

type authContextKey struct{}

type authIdentity struct {
	Actor     ticketing.Actor
	Claims    employeeClaimsPayload
	ExpiresAt time.Time
	Source    string
}

type authBootstrapResponse struct {
	MockProfilesEnabled bool                 `json:"mock_profiles_enabled"`
	MockProfiles        []mockProfilePayload `json:"mock_profiles"`
	DebugChromeEnabled  bool                 `json:"debug_chrome_enabled"`
}

type mockProfilePayload struct {
	ProfileID        string   `json:"profile_id"`
	DisplayName      string   `json:"display_name"`
	JobTitle         *string  `json:"job_title"`
	RoleClaims       []string `json:"role_claims"`
	MappedRoles      []string `json:"mapped_roles"`
	Department       string   `json:"department"`
	Site             string   `json:"site"`
	City             string   `json:"city"`
	Grade            int      `json:"grade"`
	EmploymentStatus string   `json:"employment_status"`
}

type mockProviderTokenRequest struct {
	ProfileID string `json:"profile_id"`
}

type mockProviderTokenResponse struct {
	ProviderToken string                `json:"provider_token"`
	ExpiresAt     time.Time             `json:"expires_at"`
	Claims        employeeClaimsPayload `json:"claims"`
}

var mockProviderProfiles = []providerClaims{
	{
		EmployeeID:       "E1001",
		DisplayName:      "Ariel Chen",
		JobTitle:         stringPointer("Software Engineer"),
		RoleClaims:       []string{ticketing.RoleEmployee},
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            6,
		EmploymentStatus: "active",
	},
	{
		EmployeeID:       "E1002",
		DisplayName:      "Ben Lin",
		JobTitle:         stringPointer("Product Designer"),
		RoleClaims:       []string{ticketing.RoleEmployee},
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            5,
		EmploymentStatus: "active",
	},
	{
		EmployeeID:       "E1003",
		DisplayName:      "Tainan User",
		JobTitle:         stringPointer("Field Engineer"),
		RoleClaims:       []string{ticketing.RoleEmployee},
		Department:       "Engineering",
		Site:             "Tainan HQ",
		City:             "Tainan",
		Grade:            5,
		EmploymentStatus: "active",
	},
	{
		EmployeeID:       "E2001",
		DisplayName:      "Carla Wu",
		JobTitle:         stringPointer("Account Manager"),
		RoleClaims:       []string{ticketing.RoleEmployee},
		Department:       "Sales",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            4,
		EmploymentStatus: "active",
	},
	{
		EmployeeID:       "admin-1",
		DisplayName:      "Admin One",
		JobTitle:         stringPointer("Activity Owner"),
		RoleClaims:       []string{ticketing.RoleActivityAdmin},
		Department:       "Welfare Committee",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            7,
		EmploymentStatus: "active",
	},
	{
		EmployeeID:       "staff-1",
		DisplayName:      "Staff One",
		JobTitle:         stringPointer("Check-in Staff"),
		RoleClaims:       []string{ticketing.RoleCheckinStaff},
		Department:       "Operations",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            5,
		EmploymentStatus: "active",
	},
	{
		EmployeeID:       "hr-1",
		DisplayName:      "HR One",
		JobTitle:         stringPointer("HR Partner"),
		RoleClaims:       []string{ticketing.RoleHRAdmin},
		Department:       "Human Resources",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            6,
		EmploymentStatus: "active",
	},
	{
		EmployeeID:       "system-1",
		DisplayName:      "System One",
		JobTitle:         stringPointer("System Administrator"),
		RoleClaims:       []string{ticketing.RoleSystemAdmin},
		Department:       "IT",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            8,
		EmploymentStatus: "active",
	},
}

func registerAuthRoutes(mux *http.ServeMux, provider *ProviderVerifier, appEnv string, logger *slog.Logger) {
	mux.HandleFunc("GET /api/v1/auth/me", handleMe(provider))
	mux.HandleFunc("GET /api/v1/auth/bootstrap", handleAuthBootstrap(appEnv))
	mux.HandleFunc("POST /api/v1/auth/mock-provider-token", handleMockProviderToken(provider, appEnv, logger))
}

func handleMe(provider *ProviderVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bearer, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		identity, status, err := providerIdentity(provider, bearer)
		if err != nil {
			writeError(w, status, authErrorMessage(status))
			return
		}
		writeJSON(w, http.StatusOK, authClaimsResponse(identity))
	}
}

func handleAuthBootstrap(appEnv string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		response := authBootstrapResponse{
			MockProfiles:       []mockProfilePayload{},
			DebugChromeEnabled: debugChromeEnabled(appEnv),
		}
		if mockProfilesEnabled(appEnv) {
			response.MockProfilesEnabled = true
			response.MockProfiles = mockProfilePayloads()
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func handleMockProviderToken(provider *ProviderVerifier, appEnv string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mockProfilesEnabled(appEnv) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		var req mockProviderTokenRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		claims, ok := mockProviderClaims(strings.TrimSpace(req.ProfileID), provider.now().UTC().Add(mockProviderTokenTTL))
		if !ok {
			writeError(w, http.StatusUnauthorized, "unknown mock provider profile")
			return
		}
		token, err := provider.Sign(claims)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "provider verifier is not configured")
			return
		}
		identity, status, err := providerIdentity(provider, token)
		if err != nil {
			writeError(w, status, authErrorMessage(status))
			return
		}
		if logger != nil {
			logger.Info("mock provider profile selected", "actor_ref", redactedActorRef(identity.Actor.ID), "actor_role", identity.Actor.Role)
		}
		writeJSON(w, http.StatusOK, mockProviderTokenResponse{
			ProviderToken: token,
			ExpiresAt:     identity.ExpiresAt,
			Claims:        identity.Claims,
		})
	}
}

func requireActor(provider *ProviderVerifier, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bearer, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		identity, status, err := providerIdentity(provider, bearer)
		if err != nil {
			writeError(w, status, authErrorMessage(status))
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, identity)))
	}
}

func actorFromRequest(r *http.Request) ticketing.Actor {
	if identity, ok := r.Context().Value(authContextKey{}).(authIdentity); ok {
		return identity.Actor
	}
	return ticketing.Actor{}
}

func mockProfilesEnabled(appEnv string) bool {
	return debugChromeEnabled(appEnv)
}

func debugChromeEnabled(appEnv string) bool {
	switch strings.ToLower(strings.TrimSpace(appEnv)) {
	case "local", "demo", "test":
		return true
	default:
		return false
	}
}

func bearerToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(header[len(prefix):]), true
}

func providerIdentity(provider *ProviderVerifier, token string) (authIdentity, int, error) {
	var identity authIdentity
	if provider == nil {
		return identity, http.StatusUnauthorized, errors.New("provider verifier is not configured")
	}
	identity, err := provider.IdentityFromToken(token)
	if errors.Is(err, errProviderRoleRejected) {
		return identity, http.StatusForbidden, err
	}
	if err != nil {
		return identity, http.StatusUnauthorized, err
	}
	return identity, http.StatusOK, nil
}

func authErrorMessage(status int) string {
	if status == http.StatusForbidden {
		return "role is not allowed"
	}
	return "authentication required"
}

func mockProfilePayloads() []mockProfilePayload {
	profiles := make([]mockProfilePayload, 0, len(mockProviderProfiles))
	for _, claims := range mockProviderProfiles {
		mappedRoles := mappedProviderRoles(claims.RoleClaims)
		profiles = append(profiles, mockProfilePayload{
			ProfileID:        claims.EmployeeID,
			DisplayName:      claims.DisplayName,
			JobTitle:         claims.JobTitle,
			RoleClaims:       append([]string(nil), claims.RoleClaims...),
			MappedRoles:      append([]string(nil), mappedRoles...),
			Department:       claims.Department,
			Site:             claims.Site,
			City:             claims.City,
			Grade:            claims.Grade,
			EmploymentStatus: claims.EmploymentStatus,
		})
	}
	return profiles
}

func mockProviderClaims(profileID string, expiresAt time.Time) (providerClaims, bool) {
	for _, claims := range mockProviderProfiles {
		if claims.EmployeeID == profileID {
			claims.RoleClaims = append([]string(nil), claims.RoleClaims...)
			claims.ExpiresAt = expiresAt.UTC().Unix()
			return claims, true
		}
	}
	return providerClaims{}, false
}

func redactedActorRef(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(actorID))
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}

func stringPointer(value string) *string {
	return &value
}
