package ticketing

import "time"

type NotificationPreferences struct {
	EmployeeID         string    `json:"employee_id"`
	EmailEnabled       bool      `json:"email_enabled"`
	InAppEnabled       bool      `json:"in_app_enabled"`
	OptedOutCategories []string  `json:"opted_out_categories"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type NotificationDelivery struct {
	DeliveryID  string    `json:"delivery_id"`
	OutboxID    string    `json:"outbox_id"`
	EmployeeRef string    `json:"employee_ref,omitempty"`
	Channel     string    `json:"channel"`
	Status      string    `json:"status"`
	Attempts    int       `json:"attempts"`
	LastError   string    `json:"last_error"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type NotificationDeliveryOpsQuery struct {
	Status   string
	Cursor   time.Time
	CursorID string
	Limit    int
}

type NotificationDeliveryOpsPage struct {
	Deliveries []NotificationDeliveryOpsRow `json:"deliveries"`
	NextCursor string                       `json:"next_cursor,omitempty"`
}

type NotificationDeliveryOpsRow struct {
	DeliveryID         string     `json:"delivery_id"`
	WorkerKind         string     `json:"worker_kind"`
	EventType          string     `json:"event_type"`
	Status             string     `json:"status"`
	RetryCount         int        `json:"retry_count"`
	LastError          string     `json:"last_error,omitempty"`
	RecipientRedacted  string     `json:"recipient_redacted,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	LastAttemptAt      *time.Time `json:"last_attempt_at,omitempty"`
	DeadLetterAt       *time.Time `json:"dead_letter_at,omitempty"`
	RetryEligible      bool       `json:"retry_eligible"`
	DeadLetterEligible bool       `json:"dead_letter_eligible"`
}
