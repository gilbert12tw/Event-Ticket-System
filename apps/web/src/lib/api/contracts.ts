export type Role = "employee" | "activity_admin" | "checkin_staff" | "hr_admin" | "system_admin";

export type Actor = {
  id: string;
  role: Role;
};

export type AuthSession = {
  actor: Actor;
  expires_at: string;
  claims: AuthMeClaims;
  source: "provider";
};

export type AuthMeClaims = {
  employee_id: string;
  display_name: string;
  job_title?: string | null;
  role_claims: string[];
  mapped_roles: Role[];
  department: string;
  site: string;
  city: string;
  claims_status: "complete" | "rejected";
};

export type AuthBootstrap = {
  mock_profiles_enabled: boolean;
  mock_profiles: MockProfile[];
};

export type MockProfile = {
  profile_id: string;
  display_name: string;
  job_title?: string | null;
  role_claims: string[];
  mapped_roles: Role[];
  department: string;
  site: string;
  city: string;
};

export type MockProviderToken = {
  provider_token: string;
  expires_at: string;
  claims: AuthMeClaims;
};

export type EmployeeProfile = {
  employee_id: string;
  full_name: string;
  department: string;
  site: string;
  job_grade: number;
  employment_status: string;
};

export type EligibilityRule = {
  rule_id?: string;
  event_id?: string;
  department: string;
  site: string;
  min_grade: number;
  employment_status: string;
  version?: number;
};

export type EligibilityRuleInput = {
  department: string;
  site: string;
  min_grade: number;
  employment_status: string;
};

export type EligibilityCheckResult = {
  event_id: string;
  employee_id: string;
  eligible: boolean;
  reason: string;
};

export type EligibilityPreviewRequest = {
  rule: EligibilityRuleInput;
};

export type EligibilityPreviewResponse = {
  event_id: string;
  match_count: number;
  zero_match: boolean;
};

export type UpdateEligibilityRequest = {
  rule: EligibilityRuleInput;
  allow_zero_match: boolean;
};

export type EligibilityImpactReview = {
  review_id: string;
  event_id: string;
  employee_id: string;
  ticket_id: string;
  status: string;
  reason: string;
  created_at: string;
  resolved_at?: string;
};

export type ResolveImpactReviewRequest = {
  reason: string;
};

export type EventSummary = {
  event_id: string;
  title: string;
  description: string;
  location: string;
  event_city?: string;
  event_site?: string;
  starts_at: string;
  registration_start: string;
  registration_close: string;
  capacity_type: "limited" | "unlimited";
  capacity: number | null;
  allows_family: boolean;
  status: string;
  allocation_mode: string;
  category?: string;
  tags?: string[];
  entry_method?: string;
  visibility?: string;
  version?: number;
  archived_at?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  rule: EligibilityRule;
  eligible: boolean;
  eligibility_reason: string;
  confirmed_count: number;
  waitlist_count: number;
  remaining_capacity: number | null;
  current_user_status: string;
  current_user_registration_id?: string;
  current_user_ticket?: Ticket;
  no_show_cooldown?: NoShowCooldown;
};

export type NoShowCooldown = {
  active: boolean;
  applies_to?: "limited" | "";
  until?: string | null;
  reason?: string;
};

export type Ticket = {
  ticket_id: string;
  registration_id: string;
  event_id: string;
  employee_id: string;
  status: string;
  sequence_number?: number;
  signed_token?: string;
  qr_payload?: string;
  expires_at?: string;
  revoked_reason?: string;
  issued_at: string;
  event_title?: string;
  event_location?: string;
  event_starts_at?: string;
  employee_name?: string;
  family_count?: number;
};

export type BookingResponse = {
  registration: {
    registration_id: string;
    event_id: string;
    employee_id: string;
    status: string;
    idempotency_key: string;
    cancel_idempotency_key?: string;
    cancelled_at?: string;
    cancel_reason?: string;
    family_count?: number;
    created_at: string;
  };
  ticket?: Ticket;
  remaining_capacity: number;
  message: string;
};

export type RegistrationDetail = {
  registration_id: string;
  event_id: string;
  employee_id: string;
  status: string;
  idempotency_key: string;
  cancel_idempotency_key?: string;
  cancelled_at?: string;
  cancel_reason?: string;
  family_count?: number;
  created_at: string;
  employee_name: string;
  ticket?: Ticket;
};

export type PromoteWaitlistResponse = {
  promoted?: BookingResponse;
  remaining_capacity: number;
  message: string;
};

export type CheckinResponse = {
  checkin_id: string;
  ticket_id: string;
  event_id: string;
  employee_id: string;
  status: string;
  scanned_at: string;
  first_scanned_at?: string;
  first_scanned_by?: string;
  conflict_reason?: string;
  duplicate: boolean;
};

export type OfflineTicket = {
  ticket_id: string;
  employee_id: string;
  token_hash: string;
};

export type OfflineCheckinPackage = {
  batch_id: string;
  event_id: string;
  device_id: string;
  valid_until: string;
  package_signature: string;
  ticket_count: number;
  tickets: OfflineTicket[];
};

export type OfflineCheckinScanInput = {
  signed_token: string;
  scanned_at: string;
};

export type OfflineCheckinSyncRequest = {
  batch_id: string;
  event_id: string;
  device_id: string;
  package_signature: string;
  scans: OfflineCheckinScanInput[];
};

export type OfflineCheckinSyncResponse = {
  batch_id: string;
  accepted: number;
  duplicate: number;
  conflict: number;
  results: CheckinResponse[];
};

export type NotificationPreferences = {
  employee_id: string;
  email_enabled: boolean;
  in_app_enabled: boolean;
  opted_out_categories: string[];
  updated_at: string;
};

export type UpdateNotificationPreferencesRequest = {
  email_enabled: boolean;
  in_app_enabled: boolean;
  opted_out_categories?: string[];
};

export type NotificationDelivery = {
  delivery_id: string;
  outbox_id: string;
  employee_id: string;
  channel: string;
  status: string;
  attempts: number;
  last_error: string;
  created_at: string;
  updated_at: string;
};

export type ReportRow = {
  event_id: string;
  title: string;
  capacity: number;
  confirmed_count: number;
  waitlist_count: number;
  ticket_count: number;
  checkin_count: number;
  remaining_capacity: number;
  starts_at: string;
};

export type ReportExportRequest = {
  report_type: string;
};

export type ReportExport = {
  export_id: string;
  requested_by: string;
  report_type: string;
  status: string;
  object_key: string;
  created_at: string;
  completed_at?: string;
};

export type LotteryRunRequest = {
  seed: string;
};

export type LotteryRun = {
  run_id: string;
  event_id: string;
  seed: string;
  status: string;
  winner_count: number;
  created_by: string;
  created_at: string;
};

export type AuditLog = {
  audit_id: string;
  actor_id: string;
  role: string;
  action: string;
  entity_type: string;
  entity_id: string;
  metadata: string;
  created_at: string;
};

export type CreateEventRequest = {
  title: string;
  description: string;
  location: string;
  starts_at: string;
  registration_start: string;
  registration_close: string;
  capacity: number;
  status: string;
  category?: string;
  tags?: string[];
  entry_method?: string;
  visibility?: string;
  rule: EligibilityRule;
};

export type UpdateEventRequest = {
  title?: string;
  description?: string;
  location?: string;
  starts_at?: string;
  registration_start?: string;
  registration_close?: string;
  capacity?: number;
  category?: string;
  tags?: string[];
  entry_method?: string;
  visibility?: string;
};

export type AuditLogFilters = {
  actor_id?: string;
  role?: string;
  action?: string;
  entity_type?: string;
  entity_id?: string;
  from?: string;
  to?: string;
  limit?: string;
  cursor?: string;
};

export type ApiEnvelope<T> = {
  success: boolean;
  data: T;
  error: string | null;
};

export type ApiLogEntry = {
  id: string;
  label: string;
  status: number | "ERR";
  ok: boolean;
  payload: unknown;
  createdAt: string;
};
