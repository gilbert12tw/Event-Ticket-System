package ticketing

import (
	"context"
	"strings"
	"time"
)

// OfflineCheckinPackage issues a signed offline check-in package for one event
// and device: the active ticket snapshot (token hashes only — never signing
// material), a batch row, and an HMAC package signature binding batch, event,
// device, staff, and expiry. Sync requests are validated against this batch
// row and signature in checkin_offline_service.go.
func (s *Service) OfflineCheckinPackage(ctx context.Context, actor Actor, eventID string, deviceID string) (OfflineCheckinPackage, error) {
	if err := requireRole(actor, RoleCheckinStaff); err != nil {
		return OfflineCheckinPackage{}, err
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return OfflineCheckinPackage{}, badRequest("device_id is required")
	}
	rows, err := s.db.Query(ctx, `SELECT t.ticket_id, t.employee_id, t.signed_token_hash, r.family_count, e.full_name, e.department, e.site
		FROM tickets t
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN employees e ON e.employee_id = t.employee_id
		WHERE t.event_id = $1 AND t.status = 'active'
		ORDER BY t.issued_at ASC`, eventID)
	if err != nil {
		return OfflineCheckinPackage{}, err
	}
	defer rows.Close()
	var tickets []OfflineTicket
	for rows.Next() {
		var ticket OfflineTicket
		if err := rows.Scan(&ticket.TicketID, &ticket.EmployeeID, &ticket.TokenHash, &ticket.FamilyCount, &ticket.Holder.DisplayName, &ticket.Holder.Department, &ticket.Holder.City); err != nil {
			return OfflineCheckinPackage{}, err
		}
		tickets = append(tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		return OfflineCheckinPackage{}, err
	}
	batchID, err := newID("off")
	if err != nil {
		return OfflineCheckinPackage{}, err
	}
	validUntil := s.now().Add(4 * time.Hour).UTC().Truncate(time.Second)
	signature, err := s.signer.SignOfflinePackage(OfflinePackageClaims{
		BatchID:    batchID,
		EventID:    eventID,
		DeviceID:   deviceID,
		StaffID:    actor.ID,
		ValidUntil: validUntil,
	})
	if err != nil {
		return OfflineCheckinPackage{}, err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO offline_checkin_batches
		(batch_id, event_id, device_id, staff_id, status, valid_until, package_signature)
		VALUES ($1,$2,$3,$4,'open',$5,$6)`, batchID, eventID, deviceID, actor.ID, validUntil, signature)
	if err != nil {
		return OfflineCheckinPackage{}, err
	}
	return OfflineCheckinPackage{
		BatchID:          batchID,
		EventID:          eventID,
		DeviceID:         deviceID,
		ValidUntil:       validUntil,
		PackageSignature: signature,
		TicketCount:      len(tickets),
		Tickets:          tickets,
	}, nil
}
