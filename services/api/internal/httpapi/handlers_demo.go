package httpapi

import (
	"net/http"

	"event-ticket-system/internal/ticketing"
)

func handleSeedDemo(service TicketingService, appEnv string, auth *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if appEnv != "" && appEnv != "local" && appEnv != "demo" && appEnv != "test" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		actor, _, err := auth.ActorFromCookie(r)
		if err != nil {
			if hasSessionCookie(r) {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			actor = actorFromLegacyHeaders(r)
		}
		if actor.ID == "" || actor.Role == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if actor.Role != ticketing.RoleActivityAdmin && actor.Role != ticketing.RoleHRAdmin && actor.Role != ticketing.RoleSystemAdmin {
			writeError(w, http.StatusForbidden, "role is not allowed")
			return
		}
		if err := service.SeedDemoData(r.Context()); err != nil {
			writeServiceResult(w, http.StatusOK, nil, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "seeded"})
	}
}
