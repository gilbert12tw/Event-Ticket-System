package ticketing

import "context"

func (s *Service) RevokeTicket(ctx context.Context, actor Actor, ticketID string, req RevokeTicketRequest) (Ticket, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin); err != nil {
		return Ticket{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Ticket{}, err
	}
	defer rollback(ctx, tx)

	ticket, err := s.lockTicketTx(ctx, tx, ticketID)
	if err != nil {
		return Ticket{}, err
	}
	if ticket.Status == TicketRedeemed {
		return Ticket{}, conflict("redeemed tickets cannot be revoked")
	}
	if ticket.Status != TicketRevoked {
		_, err = tx.Exec(ctx, `UPDATE tickets SET status = 'revoked', revoked_reason = $1 WHERE ticket_id = $2`, req.Reason, ticketID)
		if err != nil {
			return Ticket{}, err
		}
		ticket.Status = TicketRevoked
		ticket.RevokedReason = req.Reason
		if err := insertOutbox(ctx, tx, "ticket.revoked", ticketID, map[string]interface{}{
			"event_id":   ticket.EventID,
			"reason":     req.Reason,
			"actor_id":   actor.ID,
			"actor_role": actor.Role,
		}); err != nil {
			return Ticket{}, err
		}
		auditID, err := newID("aud")
		if err != nil {
			return Ticket{}, err
		}
		if err := insertAudit(ctx, tx, auditID, actor, "ticket.revoked", "ticket", ticketID, map[string]interface{}{"event_id": ticket.EventID, "reason": req.Reason}); err != nil {
			return Ticket{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}
