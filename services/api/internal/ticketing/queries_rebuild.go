package ticketing

// PH2-43 read-model rebuild SQL helpers.
//
// These queries rebuild reporting_event_summary from PostgreSQL OLTP truth
// (registrations + employees) rather than by replaying outbox history. They
// contain SQL only — all orchestration lives in rebuild_projection_service.go.
//
// Source of truth: registrations.status and employees.department. No OLTP
// table is ever modified here; the projection tables are the only output.
