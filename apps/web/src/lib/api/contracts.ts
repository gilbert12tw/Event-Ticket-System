export type Role =
  | "employee"
  | "activity_admin"
  | "checkin_staff"
  | "hr_admin"
  | "system_admin";

export type Actor = {
  id: string;
  role: Role;
};

type EmployeeClaims = {
  employee_id: string;
  display_name: string;
  job_title?: string | null;
  role_claims: string[];
  mapped_roles: Role[];
  department: string;
  site: string;
  city: string;
  grade: number;
  employment_status: string;
};

export type AuthSession = {
  actor: Actor;
  expires_at: string;
  claims: AuthMeClaims;
  source: "provider";
};

export type AuthMeClaims = EmployeeClaims & {
  claims_status: "complete" | "rejected";
};

export type AuthBootstrap = {
  mock_profiles_enabled: boolean;
  mock_profiles: MockProfile[];
  debug_chrome_enabled: boolean;
  demo_debug_enabled?: boolean;
  ops_api_enabled?: boolean;
};

export type MockProfile = EmployeeClaims & {
  profile_id: string;
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

export type AdminOption = {
  value: string;
  label: string;
};

export type AdminHROptions = {
  sites: AdminOption[];
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
  employee_ref: string;
  ticket_id: string;
  status: string;
  reason: string;
  created_at: string;
  resolved_at?: string;
};

export type ResolveImpactReviewRequest = {
  reason: string;
};

export type CapacityType = "limited" | "unlimited";
export type AllocationMode = "fcfs" | "lottery";

export interface EligibilityWarning {
  code: string;
  message: string;
  employee_city?: string;
  event_city?: string;
}

export interface EligibilityDecision {
  event_id: string;
  eligible: boolean;
  can_book: boolean;
  reasons: string[];
  warnings: EligibilityWarning[];
  no_show_cooldown: NoShowCooldown;
}

type EventMutableFields = {
  title: string;
  description: string;
  location: string;
  event_city?: string;
  event_site?: string;
  starts_at: string;
  registration_start: string;
  registration_close: string;
  capacity_type: CapacityType;
  capacity: number | null;
  allows_family: boolean;
  allocation_mode?: AllocationMode;
  category?: string;
  tags?: string[];
  entry_method?: string;
  visibility?: string;
};

export type EventSummary = EventMutableFields & {
  event_id: string;
  status: string;
  allocation_mode: AllocationMode;
  version?: number;
  archived_at?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  rule: EligibilityRule;
  eligibility?: EligibilityDecision;
  eligible?: boolean;
  eligibility_reason?: string;
  confirmed_count: number;
  waitlist_count: number;
  remaining_capacity: number | null;
  current_user_status: string;
  current_user_registration_id?: string;
  current_user_ticket?: Ticket;
  no_show_cooldown?: NoShowCooldown;
};

export type EventListFilters = {
  capacity_type?: CapacityType;
  city?: string;
  status?: string;
};

export type EventAsset = {
  asset_id: string;
  event_id: string;
  file_name: string;
  content_type: string;
  size_bytes: number;
  created_by: string;
  created_at: string;
};

function normalizeEligibilityDecision(
  decision: EligibilityDecision,
): EligibilityDecision {
  return {
    event_id: decision.event_id,
    eligible: decision.eligible,
    can_book: decision.can_book,
    reasons: decision.reasons ?? [],
    warnings: decision.warnings ?? [],
    no_show_cooldown: decision.no_show_cooldown ?? { active: false },
  };
}

export function getEligibilityDecision(
  event: EventSummary,
): EligibilityDecision | undefined {
  if (event.eligibility) return normalizeEligibilityDecision(event.eligibility);

  // Temporary compatibility shim. Remove after backend always returns eligibility.
  if (typeof event.eligible === "boolean") {
    return normalizeEligibilityDecision({
      event_id: event.event_id,
      eligible: event.eligible,
      can_book: event.eligible,
      reasons: event.eligibility_reason ? [event.eligibility_reason] : [],
      warnings: [],
      no_show_cooldown: event.no_show_cooldown ?? { active: false },
    });
  }

  return undefined;
}

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
  department?: string;
  city?: string;
  family_count?: number;
  non_transferable: true;
};

type RegistrationRecord = {
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

export type BookingResponse = {
  registration: RegistrationRecord;
  ticket?: Ticket;
  remaining_capacity: number;
  message: string;
  duplicate?: boolean;
};

export type RegistrationDetail = RegistrationRecord & {
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
  event_title?: string;
  employee_id: string;
  status: string;
  reason_code?: string;
  scanned_at: string;
  first_scanned_at?: string;
  first_scanned_by?: string;
  conflict_reason?: string;
  rejection_message?: string;
  duplicate: boolean;
  holder?: TicketHolder | null;
  family_count: number;
};

export type TicketHolder = {
  display_name: string;
  department: string;
  city: string;
};

export type OfflineTicket = {
  ticket_id: string;
  employee_id: string;
  token_hash: string;
  holder: TicketHolder;
  family_count: number;
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
  employee_ref?: string | null;
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
  capacity_type: CapacityType;
  capacity: number | null;
  confirmed_count: number;
  waitlist_count: number;
  employee_count: number;
  family_count: number;
  total_attendee_count: number;
  ticket_count: number;
  checkin_count: number;
  remaining_capacity: number | null;
  city_distribution: Record<string, number>;
  starts_at: string;
};

export type ReportExportRequest = {
  report_type: string;
};

export type ReportExport = {
  export_id: string;
  requested_by: string;
  report_type: string;
  format: string;
  status: string;
  object_key: string;
  created_at: string;
  completed_at?: string | null;
};

export type LotteryRunRequest = {
  seed: string;
};

export type LotteryRun = {
  run_id: string;
  event_id: string;
  seed: string;
  status: string;
  input_snapshot_at: string;
  algorithm_version: string;
  candidate_count: number;
  eligibility_rule_id: string;
  eligibility_rule_version: number;
  eligibility_snapshot: EligibilityRuleInput;
  winner_count: number;
  created_by: string;
  created_at: string;
};

export type DemoClockMode = "real" | "fixed";

export type DemoClockSnapshot = {
  enabled: boolean;
  mode: DemoClockMode;
  now: string;
  real_now: string;
  updated_at?: string;
  reason?: string;
};

export type DemoClockUpdateRequest = {
  mode: DemoClockMode;
  now?: string;
  reason: string;
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

export type CreateEventRequest = EventMutableFields & {
  status: string;
  rule: EligibilityRule;
};

export type UpdateEventRequest = Partial<EventMutableFields>;

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
  requestBody: unknown;
  responseBody: unknown;
  createdAt: string;
};
