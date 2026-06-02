package ticketing

import "context"

const (
	opsAccessDeniedAction     = "ops.access_denied"
	opsAccessDeniedEntityType = "ops_endpoint"
)

func (s *Service) requireOpsAnyRole(ctx context.Context, actor Actor, resource string, roles ...string) error {
	err := requireAnyRole(actor, roles...)
	if err == nil {
		return nil
	}
	if auditErr := s.insertOpsAccessDeniedAudit(ctx, actor, resource, err, roles); auditErr != nil {
		return auditErr
	}
	return err
}

func (s *Service) requireOpsFeedRole(ctx context.Context, actor Actor, resource string) error {
	return s.requireOpsAnyRole(ctx, actor, resource, RoleHRAdmin, RoleSystemAdmin)
}

func (s *Service) insertOpsAccessDeniedAudit(ctx context.Context, actor Actor, resource string, denial error, requiredRoles []string) error {
	if s.db == nil {
		return nil
	}
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	auditActor := actor
	if auditActor.ID == "" {
		auditActor.ID = "unknown"
	}
	if auditActor.Role == "" {
		auditActor.Role = "unknown"
	}
	return insertAuditWithExecutor(ctx, s.db, newAuditRecord(auditID, auditActor, opsAccessDeniedAction, opsAccessDeniedEntityType, resource, map[string]interface{}{
		"resource":       resource,
		"required_roles": requiredRoles,
		"status":         ErrorStatus(denial),
		"reason":         ErrorMessage(denial),
	}))
}
