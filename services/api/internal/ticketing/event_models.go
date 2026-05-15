package ticketing

import (
	"encoding/json"
	"time"
)

const (
	CapacityTypeLimited   = "limited"
	CapacityTypeUnlimited = "unlimited"
)

type Event struct {
	EventID           string    `json:"event_id"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Location          string    `json:"location"`
	EventCity         string    `json:"event_city"`
	EventSite         string    `json:"event_site"`
	StartsAt          time.Time `json:"starts_at"`
	RegistrationStart time.Time `json:"registration_start"`
	RegistrationClose time.Time `json:"registration_close"`
	CapacityType      string    `json:"capacity_type"`
	Capacity          *int      `json:"capacity"`
	AllowsFamily      bool      `json:"allows_family"`
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
	Rule                      EligibilityRule     `json:"rule"`
	Eligibility               EligibilityDecision `json:"eligibility"`
	Eligible                  bool                `json:"eligible"`
	EligibilityReason         string              `json:"eligibility_reason"`
	ConfirmedCount            int             `json:"confirmed_count"`
	WaitlistCount             int             `json:"waitlist_count"`
	RemainingCapacity         *int            `json:"remaining_capacity"`
	CurrentUserStatus         string          `json:"current_user_status"`
	CurrentUserRegistrationID string          `json:"current_user_registration_id,omitempty"`
	CurrentUserTicket         *Ticket         `json:"current_user_ticket,omitempty"`
	NoShowCooldown            NoShowCooldown  `json:"no_show_cooldown"`
}

type NoShowCooldown struct {
	Active    bool       `json:"active"`
	AppliesTo string     `json:"applies_to,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
	Reason    string     `json:"reason,omitempty"`
}

type CreateEventRequest struct {
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Location          string    `json:"location"`
	EventCity         string    `json:"event_city"`
	EventSite         string    `json:"event_site"`
	StartsAt          time.Time `json:"starts_at"`
	RegistrationStart time.Time `json:"registration_start"`
	RegistrationClose time.Time `json:"registration_close"`
	CapacityType      string    `json:"capacity_type"`
	Capacity          int       `json:"capacity"`
	AllowsFamily      bool      `json:"allows_family"`
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
	EventCity         *string    `json:"event_city,omitempty"`
	EventSite         *string    `json:"event_site,omitempty"`
	StartsAt          *time.Time `json:"starts_at,omitempty"`
	RegistrationStart *time.Time `json:"registration_start,omitempty"`
	RegistrationClose *time.Time `json:"registration_close,omitempty"`
	CapacityType      *string    `json:"capacity_type,omitempty"`
	Capacity          *int       `json:"capacity,omitempty"`
	AllowsFamily      *bool      `json:"allows_family,omitempty"`
	Category          *string    `json:"category,omitempty"`
	Tags              []string   `json:"tags,omitempty"`
	EntryMethod       *string    `json:"entry_method,omitempty"`
	Visibility        *string    `json:"visibility,omitempty"`
	capacitySet       bool
}

func (r *CreateEventRequest) UnmarshalJSON(data []byte) error {
	type alias CreateEventRequest
	var raw struct {
		alias
		RegistrationOpensAt  *time.Time      `json:"registration_opens_at"`
		RegistrationClosesAt *time.Time      `json:"registration_closes_at"`
		EligibilityRule      *RuleInput      `json:"eligibility_rule"`
		CapacityRaw          json.RawMessage `json:"capacity"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = CreateEventRequest(raw.alias)
	if raw.RegistrationOpensAt != nil {
		r.RegistrationStart = *raw.RegistrationOpensAt
	}
	if raw.RegistrationClosesAt != nil {
		r.RegistrationClose = *raw.RegistrationClosesAt
	}
	if raw.EligibilityRule != nil {
		r.Rule = *raw.EligibilityRule
	}
	if raw.CapacityRaw != nil {
		if string(raw.CapacityRaw) != "null" {
			if err := json.Unmarshal(raw.CapacityRaw, &r.Capacity); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *UpdateEventRequest) UnmarshalJSON(data []byte) error {
	type alias UpdateEventRequest
	var raw struct {
		alias
		RegistrationOpensAt  *time.Time      `json:"registration_opens_at"`
		RegistrationClosesAt *time.Time      `json:"registration_closes_at"`
		CapacityRaw          json.RawMessage `json:"capacity"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = UpdateEventRequest(raw.alias)
	if raw.RegistrationOpensAt != nil {
		r.RegistrationStart = raw.RegistrationOpensAt
	}
	if raw.RegistrationClosesAt != nil {
		r.RegistrationClose = raw.RegistrationClosesAt
	}
	if raw.CapacityRaw != nil {
		r.capacitySet = true
		if string(raw.CapacityRaw) == "null" {
			r.Capacity = nil
		} else {
			var capacity int
			if err := json.Unmarshal(raw.CapacityRaw, &capacity); err != nil {
				return err
			}
			r.Capacity = &capacity
		}
	}
	return nil
}

type ChangeEventStateRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}
