package ticketing

import "time"

type CheckinRequest struct {
	SignedToken          string `json:"signed_token"`
	EventID              string `json:"event_id,omitempty"`
	DeviceID             string `json:"device_id"`
	HolderMismatchReason string `json:"holder_mismatch_reason,omitempty"`
}

type CheckinResponse struct {
	CheckinID        string       `json:"checkin_id"`
	TicketID         string       `json:"ticket_id"`
	EventID          string       `json:"event_id"`
	EmployeeID       string       `json:"employee_id"`
	Status           string       `json:"status"`
	ReasonCode       string       `json:"reason_code,omitempty"`
	ScannedAt        time.Time    `json:"scanned_at"`
	FirstScannedAt   time.Time    `json:"first_scanned_at,omitempty"`
	FirstScannedBy   string       `json:"first_scanned_by,omitempty"`
	ConflictReason   string       `json:"conflict_reason,omitempty"`
	RejectionMessage string       `json:"rejection_message,omitempty"`
	Duplicate        bool         `json:"duplicate"`
	Holder           TicketHolder `json:"holder,omitempty"`
	FamilyCount      int          `json:"family_count"`
}

type OfflineCheckinPackage struct {
	BatchID          string          `json:"batch_id"`
	EventID          string          `json:"event_id"`
	DeviceID         string          `json:"device_id"`
	ValidUntil       time.Time       `json:"valid_until"`
	PackageSignature string          `json:"package_signature"`
	TicketCount      int             `json:"ticket_count"`
	Tickets          []OfflineTicket `json:"tickets"`
}

type OfflineTicket struct {
	TicketID    string       `json:"ticket_id"`
	EmployeeID  string       `json:"employee_id"`
	TokenHash   string       `json:"token_hash"`
	Holder      TicketHolder `json:"holder"`
	FamilyCount int          `json:"family_count"`
}

type OfflineCheckinSyncRequest struct {
	BatchID          string                    `json:"batch_id"`
	EventID          string                    `json:"event_id"`
	DeviceID         string                    `json:"device_id"`
	PackageSignature string                    `json:"package_signature"`
	Scans            []OfflineCheckinScanInput `json:"scans"`
}

type OfflineCheckinScanInput struct {
	SignedToken string    `json:"signed_token"`
	ScannedAt   time.Time `json:"scanned_at"`
}

type OfflineCheckinSyncResponse struct {
	BatchID   string            `json:"batch_id"`
	Accepted  int               `json:"accepted"`
	Duplicate int               `json:"duplicate"`
	Conflict  int               `json:"conflict"`
	Results   []CheckinResponse `json:"results"`
}
