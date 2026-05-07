package ticketing

import "time"

type AuditLogQuery struct {
	ActorID    string
	Role       string
	Action     string
	EntityType string
	EntityID   string
	From       time.Time
	To         time.Time
	Limit      int
	Cursor     time.Time
	CursorID   string
}

type AuditLog struct {
	AuditID    string    `json:"audit_id"`
	ActorID    string    `json:"actor_id"`
	Role       string    `json:"role"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Metadata   string    `json:"metadata"`
	CreatedAt  time.Time `json:"created_at"`
}
