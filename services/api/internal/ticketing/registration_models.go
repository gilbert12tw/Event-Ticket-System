package ticketing

import "time"

type Registration struct {
	RegistrationID string    `json:"registration_id"`
	EventID        string    `json:"event_id"`
	EmployeeID     string    `json:"employee_id"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	CancelKey      string    `json:"cancel_idempotency_key,omitempty"`
	CancelledAt    time.Time `json:"cancelled_at,omitempty"`
	CancelReason   string    `json:"cancel_reason,omitempty"`
	FamilyCount    int       `json:"family_count"`
	CreatedAt      time.Time `json:"created_at"`
}

type CancelRegistrationRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

type RegistrationDetail struct {
	Registration
	EmployeeName string  `json:"employee_name"`
	Ticket       *Ticket `json:"ticket,omitempty"`
}

type PromoteWaitlistResponse struct {
	Promoted          *BookingResponse `json:"promoted,omitempty"`
	RemainingCapacity int              `json:"remaining_capacity"`
	Message           string           `json:"message"`
}

type BookingRequest struct {
	EmployeeID     string `json:"employee_id"`
	IdempotencyKey string `json:"idempotency_key"`
	FamilyCount    int    `json:"family_count"`
}

type BookingResponse struct {
	Registration      Registration `json:"registration"`
	Ticket            *Ticket      `json:"ticket,omitempty"`
	RemainingCapacity int          `json:"remaining_capacity"`
	Message           string       `json:"message"`
	Duplicate         bool         `json:"duplicate,omitempty"`
}
