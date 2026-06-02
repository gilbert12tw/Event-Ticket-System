package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"event-ticket-system/internal/ticketing"
)

const demoClockReasonMaxLength = 300

type demoClockUpdateRequest struct {
	Mode   string     `json:"mode"`
	Now    *time.Time `json:"now"`
	Reason string     `json:"reason"`
}

func registerDemoDebugRoutes(mux *http.ServeMux, provider *ProviderVerifier, clock *ticketing.DemoClock, logger *slog.Logger) {
	if clock == nil {
		return
	}
	protected := func(next http.HandlerFunc) http.HandlerFunc {
		return requireActor(provider, next)
	}
	mux.HandleFunc("GET /api/v1/debug/demo-clock", protected(handleGetDemoClock(clock)))
	mux.HandleFunc("PUT /api/v1/debug/demo-clock", protected(handleUpdateDemoClock(clock, logger)))
}

func handleGetDemoClock(clock *ticketing.DemoClock) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !demoClockAllowed(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, clock.Snapshot())
	}
}

func handleUpdateDemoClock(clock *ticketing.DemoClock, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !demoClockAllowed(w, r) {
			return
		}
		var req demoClockUpdateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		reason, reasonErr := normalizeDemoClockReason(req.Reason)
		if reasonErr != "" {
			writeError(w, http.StatusBadRequest, reasonErr)
			return
		}

		var snapshot ticketing.DemoClockSnapshot
		mode := strings.ToLower(strings.TrimSpace(req.Mode))
		switch mode {
		case ticketing.DemoClockModeReal:
			snapshot = clock.UseReal(reason)
		case ticketing.DemoClockModeFixed:
			if req.Now == nil || req.Now.IsZero() {
				writeError(w, http.StatusBadRequest, "now is required when demo clock mode is fixed")
				return
			}
			snapshot = clock.UseFixed(*req.Now, reason)
		default:
			writeError(w, http.StatusBadRequest, "mode must be real or fixed")
			return
		}

		actor := actorFromRequest(r)
		if logger != nil {
			logger.Info("demo clock updated",
				"actor_ref", redactedActorRef(actor.ID),
				"actor_role", actor.Role,
				"mode", snapshot.Mode,
				"demo_now", snapshot.Now.Format(time.RFC3339),
				"reason", snapshot.Reason,
			)
		}
		writeJSON(w, http.StatusOK, snapshot)
	}
}

func normalizeDemoClockReason(value string) (string, string) {
	reason := strings.TrimSpace(value)
	if reason == "" {
		return "", "reason is required"
	}
	if utf8.RuneCountInString(reason) > demoClockReasonMaxLength {
		return "", "reason must be 300 characters or fewer"
	}
	return reason, ""
}

func demoClockAllowed(w http.ResponseWriter, r *http.Request) bool {
	role := actorFromRequest(r).Role
	if role != ticketing.RoleActivityAdmin && role != ticketing.RoleSystemAdmin {
		writeError(w, http.StatusForbidden, "role is not allowed")
		return false
	}
	return true
}
