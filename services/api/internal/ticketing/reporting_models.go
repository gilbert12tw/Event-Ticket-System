package ticketing

import "time"

const (
	ReportExportStatusPending     = "pending"
	ReportExportStatusReady       = "ready"
	ReportExportStatusFailed      = "failed"
	ReportExportTypeParticipation = "participation"
	ReportExportFormatCSV         = "csv"
)

type ReportExportRequest struct {
	ReportType string `json:"report_type"`
	Format     string `json:"format,omitempty"`
}

type ReportExport struct {
	ExportID    string     `json:"export_id"`
	RequestedBy string     `json:"requested_by"`
	ReportType  string     `json:"report_type"`
	Format      string     `json:"format"`
	Status      string     `json:"status"`
	ObjectKey   string     `json:"object_key"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type LotteryRunRequest struct {
	Seed string `json:"seed"`
}

type LotteryRun struct {
	RunID                  string    `json:"run_id"`
	EventID                string    `json:"event_id"`
	Seed                   string    `json:"seed"`
	Status                 string    `json:"status"`
	InputSnapshotAt        time.Time `json:"input_snapshot_at"`
	AlgorithmVersion       string    `json:"algorithm_version"`
	CandidateCount         int       `json:"candidate_count"`
	EligibilityRuleID      string    `json:"eligibility_rule_id"`
	EligibilityRuleVersion int       `json:"eligibility_rule_version"`
	EligibilitySnapshot    RuleInput `json:"eligibility_snapshot"`
	WinnerCount            int       `json:"winner_count"`
	CreatedBy              string    `json:"created_by"`
	CreatedAt              time.Time `json:"created_at"`
}

type ReportRow struct {
	EventID            string         `json:"event_id"`
	Title              string         `json:"title"`
	CapacityType       string         `json:"capacity_type"`
	Capacity           *int           `json:"capacity"`
	ConfirmedCount     int            `json:"confirmed_count"`
	WaitlistCount      int            `json:"waitlist_count"`
	EmployeeCount      int            `json:"employee_count"`
	FamilyCount        int            `json:"family_count"`
	TotalAttendeeCount int            `json:"total_attendee_count"`
	TicketCount        int            `json:"ticket_count"`
	CheckinCount       int            `json:"checkin_count"`
	RemainingCapacity  *int           `json:"remaining_capacity"`
	CityDistribution   map[string]int `json:"city_distribution"`
	StartsAt           time.Time      `json:"starts_at"`
}
