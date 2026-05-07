package ticketing

import "time"

type Event struct {
	EventID           string    `json:"event_id"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Location          string    `json:"location"`
	StartsAt          time.Time `json:"starts_at"`
	RegistrationStart time.Time `json:"registration_start"`
	RegistrationClose time.Time `json:"registration_close"`
	Capacity          int       `json:"capacity"`
	Status            string    `json:"status"`
	AllocationMode    string    `json:"allocation_mode"`
	Category          string    `json:"category"`
	Tags              []string  `json:"tags"`
	EntryMethod       string    `json:"entry_method"`
	Visibility        string    `json:"visibility"`
	Version           int       `json:"version"`
	ArchivedAt        time.Time `json:"archived_at,omitempty"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type EventSummary struct {
	Event
	Rule              EligibilityRule `json:"rule"`
	Eligible          bool            `json:"eligible"`
	EligibilityReason string          `json:"eligibility_reason"`
	ConfirmedCount    int             `json:"confirmed_count"`
	WaitlistCount     int             `json:"waitlist_count"`
	RemainingCapacity int             `json:"remaining_capacity"`
	CurrentUserStatus string          `json:"current_user_status"`
	CurrentUserTicket *Ticket         `json:"current_user_ticket,omitempty"`
}

type CreateEventRequest struct {
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Location          string    `json:"location"`
	StartsAt          time.Time `json:"starts_at"`
	RegistrationStart time.Time `json:"registration_start"`
	RegistrationClose time.Time `json:"registration_close"`
	Capacity          int       `json:"capacity"`
	Status            string    `json:"status"`
	Category          string    `json:"category"`
	Tags              []string  `json:"tags"`
	EntryMethod       string    `json:"entry_method"`
	Visibility        string    `json:"visibility"`
	Rule              RuleInput `json:"rule"`
}

type UpdateEventRequest struct {
	Title             *string    `json:"title,omitempty"`
	Description       *string    `json:"description,omitempty"`
	Location          *string    `json:"location,omitempty"`
	StartsAt          *time.Time `json:"starts_at,omitempty"`
	RegistrationStart *time.Time `json:"registration_start,omitempty"`
	RegistrationClose *time.Time `json:"registration_close,omitempty"`
	Capacity          *int       `json:"capacity,omitempty"`
	Category          *string    `json:"category,omitempty"`
	Tags              []string   `json:"tags,omitempty"`
	EntryMethod       *string    `json:"entry_method,omitempty"`
	Visibility        *string    `json:"visibility,omitempty"`
}

type ChangeEventStateRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}
