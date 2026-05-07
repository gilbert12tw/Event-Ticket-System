package ticketing

import "time"

type Ticket struct {
	TicketID       string    `json:"ticket_id"`
	RegistrationID string    `json:"registration_id"`
	EventID        string    `json:"event_id"`
	EmployeeID     string    `json:"employee_id"`
	Status         string    `json:"status"`
	SequenceNumber int       `json:"sequence_number"`
	SignedToken    string    `json:"signed_token,omitempty"`
	QRPayload      string    `json:"qr_payload,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	RevokedReason  string    `json:"revoked_reason,omitempty"`
	IssuedAt       time.Time `json:"issued_at"`
	EventTitle     string    `json:"event_title,omitempty"`
	EventLocation  string    `json:"event_location,omitempty"`
	EventStartsAt  time.Time `json:"event_starts_at,omitempty"`
	EmployeeName   string    `json:"employee_name,omitempty"`
}

type RevokeTicketRequest struct {
	Reason string `json:"reason"`
}
