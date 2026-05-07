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
	ReviewID   string    `json:"review_id"`
	EventID    string    `json:"event_id"`
	EmployeeID string    `json:"employee_id"`
	TicketID   string    `json:"ticket_id"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

type ResolveImpactReviewRequest struct {
	Reason string `json:"reason"`
}
