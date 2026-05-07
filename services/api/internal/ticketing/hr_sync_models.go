package ticketing

import "time"

type HRSyncRequest struct {
	Source string `json:"source"`
}

type HRSyncBatch struct {
	BatchID       string    `json:"batch_id"`
	Source        string    `json:"source"`
	Status        string    `json:"status"`
	EmployeeCount int       `json:"employee_count"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
}
