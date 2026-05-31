package httpapi

import (
	"net/http"

	"event-ticket-system/internal/ticketing"
)

func handleSeedDemo(service TicketingService, appEnv string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if appEnv != "" && appEnv != "local" && appEnv != "demo" && appEnv != "test" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		actor := actorFromRequest(r)
		if actor.ID == "" || actor.Role == "" {
			writeError(w, http.StatusUnauthorized, authRequiredMessage)
			return
		}
		if actor.Role != ticketing.RoleActivityAdmin && actor.Role != ticketing.RoleHRAdmin && actor.Role != ticketing.RoleSystemAdmin {
			writeError(w, http.StatusForbidden, authRoleNotAllowed)
			return
		}
		if err := service.SeedDemoData(r.Context()); err != nil {
			writeServiceResult(w, http.StatusOK, nil, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "seeded"})
	}
}
