package ticketing

import "time"

type EligibilityRule struct {
	RuleID           string `json:"rule_id"`
	EventID          string `json:"event_id"`
	Department       string `json:"department"`
	Site             string `json:"site"`
	MinGrade         int    `json:"min_grade"`
	EmploymentStatus string `json:"employment_status"`
	Version          int    `json:"version"`
}

type RuleInput struct {
	Department       string `json:"department"`
	Site             string `json:"site"`
	MinGrade         int    `json:"min_grade"`
	EmploymentStatus string `json:"employment_status"`
}

type EligibilityPreviewRequest struct {
	Rule RuleInput `json:"rule"`
}

type EligibilityPreviewResponse struct {
	EventID    string `json:"event_id"`
	MatchCount int    `json:"match_count"`
	ZeroMatch  bool   `json:"zero_match"`
}

type UpdateEligibilityRequest struct {
	Rule           RuleInput `json:"rule"`
	AllowZeroMatch bool      `json:"allow_zero_match"`
}

type EligibilityImpactReview struct {
	ReviewID    string    `json:"review_id"`
	EventID     string    `json:"event_id"`
	EmployeeRef string    `json:"employee_ref"`
	TicketID    string    `json:"ticket_id"`
	Status      string    `json:"status"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
	ResolvedAt  time.Time `json:"resolved_at,omitempty"`
}

type ResolveImpactReviewRequest struct {
	Reason string `json:"reason"`
}

// WarningCode identifies a non-blocking advisory returned alongside an
// eligibility decision.
type WarningCode string

const (
	// WarningCrossCity is emitted when the employee's provider city differs
	// from the event city. It is advisory — it must never set Eligible=false
	// or CanBook=false on its own.
	WarningCrossCity WarningCode = "cross_city"
)

// EligibilityWarning is a structured, non-blocking advisory. It must never
// prevent booking on its own.
type EligibilityWarning struct {
	Code         WarningCode `json:"code"`
	Message      string      `json:"message"`
	EmployeeCity string      `json:"employee_city,omitempty"`
	EventCity    string      `json:"event_city,omitempty"`
}

// EligibilityDecision is the response returned to an employee for a single
// event eligibility check. It replaces the previous map[string]interface{}
// returned by CheckEligibility.
type EligibilityDecision struct {
	EventID        string               `json:"event_id"`
	Eligible       bool                 `json:"eligible"`
	CanBook        bool                 `json:"can_book"`
	Reasons        []string             `json:"reasons"`
	Warnings       []EligibilityWarning `json:"warnings"`
	NoShowCooldown NoShowCooldown       `json:"no_show_cooldown"`
}
