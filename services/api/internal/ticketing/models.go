package ticketing

const (
	RoleEmployee      = "employee"
	RoleActivityAdmin = "activity_admin"
	RoleCheckinStaff  = "checkin_staff"
	RoleHRAdmin       = "hr_admin"
	RoleSystemAdmin   = "system_admin"

	EventStatusPublished = "published"
	EventStatusDraft     = "draft"
	EventStatusClosed    = "closed"
	EventStatusCancelled = "cancelled"
	EventStatusArchived  = "archived"

	RegistrationConfirmed  = "confirmed"
	RegistrationWaitlisted = "waitlisted"
	RegistrationCancelled  = "cancelled"

	TicketActive   = "active"
	TicketRedeemed = "redeemed"
	TicketRevoked  = "revoked"
	TicketExpired  = "expired"
)

type Actor struct {
	ID   string
	Role string
}

type Employee struct {
	EmployeeID       string `json:"employee_id"`
	FullName         string `json:"full_name"`
	Department       string `json:"department"`
	Site             string `json:"site"`
	JobGrade         int    `json:"job_grade"`
	EmploymentStatus string `json:"employment_status"`
}
