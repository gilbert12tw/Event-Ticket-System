package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"event-ticket-system/internal/ticketing"
)

const sessionCookieName = "cets_session"

type AuthConfig struct {
	Secret       string
	TTL          time.Duration
	CookieSecure bool
	AppEnv       string
}

type SessionManager struct {
	secret       []byte
	ttl          time.Duration
	cookieSecure bool
	allowLocal   bool
	now          func() time.Time
}

type authContextKey struct{}

type sessionClaims struct {
	Subject   string `json:"subject"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
}

type loginRequest struct {
	PrincipalID string `json:"principal_id"`
}

type authActor struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type authSessionResponse struct {
	Actor     authActor `json:"actor"`
	ExpiresAt time.Time `json:"expires_at"`
}

var localSSOPrincipals = map[string]ticketing.Actor{
	"E1001":    {ID: "E1001", Role: ticketing.RoleEmployee},
	"E1002":    {ID: "E1002", Role: ticketing.RoleEmployee},
	"E2001":    {ID: "E2001", Role: ticketing.RoleEmployee},
	"admin-1":  {ID: "admin-1", Role: ticketing.RoleActivityAdmin},
	"staff-1":  {ID: "staff-1", Role: ticketing.RoleCheckinStaff},
	"hr-1":     {ID: "hr-1", Role: ticketing.RoleHRAdmin},
	"system-1": {ID: "system-1", Role: ticketing.RoleSystemAdmin},
}

func NewSessionManager(config AuthConfig) *SessionManager {
	if strings.TrimSpace(config.Secret) == "" {
		config.Secret = "local-dev-auth-session-secret"
	}
	if config.TTL <= 0 {
		config.TTL = 8 * time.Hour
	}
	appEnv := strings.TrimSpace(config.AppEnv)
	if appEnv == "" {
		appEnv = strings.TrimSpace(os.Getenv("APP_ENV"))
	}
	return &SessionManager{
		secret:       []byte(config.Secret),
		ttl:          config.TTL,
		cookieSecure: config.CookieSecure,
		allowLocal:   allowLocalAuth(appEnv),
		now:          time.Now,
	}
}

func registerAuthRoutes(mux *http.ServeMux, auth *SessionManager, logger *slog.Logger) {
	mux.HandleFunc("POST /api/v1/auth/login", handleLogin(auth, logger))
	mux.HandleFunc("GET /api/v1/auth/me", handleMe(auth))
	mux.HandleFunc("POST /api/v1/auth/logout", handleLogout(auth, logger))
}

func handleLogin(auth *SessionManager, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.allowLocalLogin() {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		var req loginRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		actor, ok := localSSOPrincipals[strings.TrimSpace(req.PrincipalID)]
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid local SSO principal")
			return
		}

		token, expiresAt, err := auth.Sign(actor)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create session")
			return
		}
		auth.SetCookie(w, token, expiresAt)
		if logger != nil {
			logger.Info("local sso login", "actor_ref", redactedActorRef(actor.ID), "actor_role", actor.Role)
		}
		writeJSON(w, http.StatusOK, authResponse(actor, expiresAt))
	}
}

func handleMe(auth *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, expiresAt, err := auth.ActorFromCookie(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		writeJSON(w, http.StatusOK, authResponse(actor, expiresAt))
	}
}

func handleLogout(auth *SessionManager, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if actor, _, err := auth.ActorFromCookie(r); err == nil && logger != nil {
			logger.Info("local sso logout", "actor_ref", redactedActorRef(actor.ID), "actor_role", actor.Role)
		}
		auth.ClearCookie(w)
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	}
}

func requireActor(auth *SessionManager, appEnv string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _, err := auth.ActorFromCookie(r)
		if err == nil {
			next(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, actor)))
			return
		}
		if hasSessionCookie(r) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if allowLegacyHeaders(appEnv) {
			legacy := actorFromLegacyHeaders(r)
			if legacy.ID != "" && legacy.Role != "" {
				next(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, legacy)))
				return
			}
		}
		writeError(w, http.StatusUnauthorized, "authentication required")
	}
}

func actorFromRequest(r *http.Request) ticketing.Actor {
	if actor, ok := r.Context().Value(authContextKey{}).(ticketing.Actor); ok {
		return actor
	}
	return actorFromLegacyHeaders(r)
}

func actorFromLegacyHeaders(r *http.Request) ticketing.Actor {
	return ticketing.Actor{
		ID:   strings.TrimSpace(r.Header.Get("X-Actor-ID")),
		Role: strings.TrimSpace(r.Header.Get("X-Role")),
	}
}

func allowLegacyHeaders(appEnv string) bool {
	return allowLocalAuth(appEnv)
}

func allowLocalAuth(appEnv string) bool {
	switch strings.ToLower(strings.TrimSpace(appEnv)) {
	case "local", "demo", "test":
		return true
	default:
		return false
	}
}

func hasSessionCookie(r *http.Request) bool {
	_, err := r.Cookie(sessionCookieName)
	return err == nil
}

func authResponse(actor ticketing.Actor, expiresAt time.Time) authSessionResponse {
	return authSessionResponse{
		Actor:     authActor{ID: actor.ID, Role: actor.Role},
		ExpiresAt: expiresAt.UTC(),
	}
}

func redactedActorRef(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(actorID))
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}

func (m *SessionManager) allowLocalLogin() bool {
	return m.allowLocal
}

func (m *SessionManager) Sign(actor ticketing.Actor) (string, time.Time, error) {
	now := m.now().UTC()
	expiresAt := now.Add(m.ttl)
	claims := sessionClaims{
		Subject:   actor.ID,
		Role:      actor.Role,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	return payloadPart + "." + m.signature(payloadPart), expiresAt, nil
}

func (m *SessionManager) ActorFromCookie(r *http.Request) (ticketing.Actor, time.Time, error) {
	var actor ticketing.Actor
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return actor, time.Time{}, err
	}
	claims, err := m.Verify(cookie.Value)
	if err != nil {
		return actor, time.Time{}, err
	}
	return ticketing.Actor{ID: claims.Subject, Role: claims.Role}, time.Unix(claims.ExpiresAt, 0).UTC(), nil
}

func (m *SessionManager) Verify(token string) (sessionClaims, error) {
	var claims sessionClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, errors.New("invalid session format")
	}
	expected := m.signature(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return claims, errors.New("invalid session signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, err
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, err
	}
	if claims.Subject == "" || claims.Role == "" || claims.ExpiresAt <= 0 {
		return claims, errors.New("session claims are incomplete")
	}
	if m.now().UTC().Unix() >= claims.ExpiresAt {
		return claims, errors.New("session expired")
	}
	return claims, nil
}

func (m *SessionManager) SetCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.cookieSecure,
	})
}

func (m *SessionManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.cookieSecure,
	})
}

func (m *SessionManager) signature(payloadPart string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
