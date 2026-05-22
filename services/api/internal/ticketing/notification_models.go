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
