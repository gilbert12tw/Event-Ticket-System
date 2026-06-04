# frozen_string_literal: true

require "pathname"
require "yaml"

ROOT = Pathname.new(__dir__).join("..").expand_path
OPENAPI_ROOT = ROOT.join("docs/openapi.yaml")
OPENAPI_SOURCE_GLOB = ROOT.join("docs/openapi/**/*.yaml").to_s

require_relative "openapi_contract_helpers"

HTTP_METHODS = %w[get put post delete options head patch trace].freeze
BANNED_PATHS = [
  "/auth/login",
  "/auth/logout",
  "/employees/{employee_id}/tickets",
  "/admin/ops/queues/{kind}/replay"
].freeze

ADMIN_EVENTS_PATH = "/admin/events"
ADMIN_EVENT_PATH = "/admin/events/{event_id}"
CHECKINS_PATH = "/checkins"
NOTIFICATION_PREFERENCES_PATH = "/notifications/preferences"
DEBUG_DEMO_CLOCK_PATH = "/debug/demo-clock"
EVENT_POSTER_PATH = "/events/{event_id}/poster"
REPORT_EXPORT_DOWNLOAD_PATH = "/admin/reports/exports/{export_id}/download"

EXPECTED_OPERATIONS = {
  "/auth/me" => %w[get],
  "/auth/bootstrap" => %w[get],
  DEBUG_DEMO_CLOCK_PATH => %w[get put],
  ADMIN_EVENTS_PATH => %w[get post],
  "/events" => %w[get],
  "/events/{event_id}" => %w[get],
  EVENT_POSTER_PATH => %w[get],
  "/events/{event_id}/eligibility" => %w[get],
  "/events/{event_id}/bookings" => %w[post],
  "/me/tickets" => %w[get],
  "/me/registrations/{registration_id}/cancel" => %w[post],
  ADMIN_EVENT_PATH => %w[patch delete],
  "/admin/events/{event_id}/poster" => %w[post],
  "/admin/events/{event_id}/state" => %w[post],
  "/admin/events/{event_id}/duplicate" => %w[post],
  "/admin/events/{event_id}/eligibility/preview" => %w[post],
  "/admin/events/{event_id}/eligibility" => %w[put],
  "/admin/eligibility-impact-reviews" => %w[get],
  "/admin/eligibility-impact-reviews/{review_id}/resolve" => %w[post],
  "/admin/events/{event_id}/registrations" => %w[get],
  "/admin/events/{event_id}/registrations/{registration_id}/cancel" => %w[post],
  "/admin/events/{event_id}/waitlist/promote" => %w[post],
  "/admin/events/{event_id}/bans/{target_employee_id}" => %w[delete],
  "/admin/events/{event_id}/lottery-runs" => %w[post],
  "/tickets/{ticket_id}" => %w[get],
  "/admin/tickets/{ticket_id}/revoke" => %w[post],
  CHECKINS_PATH => %w[post],
  "/checkins/events/{event_id}/offline-package" => %w[get],
  "/checkins/offline-sync" => %w[post],
  NOTIFICATION_PREFERENCES_PATH => %w[get put],
  "/admin/notifications/deliveries" => %w[get],
  "/admin/notifications/deliveries/{delivery_id}/retry" => %w[post],
  "/admin/reports" => %w[get],
  "/admin/reports/exports" => %w[post],
  "/admin/reports/exports/{export_id}" => %w[get],
  REPORT_EXPORT_DOWNLOAD_PATH => %w[get],
  "/admin/hr/options" => %w[get],
  "/admin/audit-logs" => %w[get],
  "/admin/ops/capacity-pressure" => %w[get],
  "/admin/ops/queues" => %w[get],
  "/admin/ops/notification-deliveries" => %w[get],
  "/admin/ops/report-freshness" => %w[get],
  "/admin/ops/dashboard" => %w[get]
}.freeze

PUBLIC_OPERATIONS = [
  ["get", "/auth/bootstrap"]
].freeze

BINARY_OPERATIONS = [
  ["get", EVENT_POSTER_PATH],
  ["get", REPORT_EXPORT_DOWNLOAD_PATH]
].freeze

EXPECTED_REQUIRED_ROLES = {
  ["get", "/auth/me"] => %w[employee activity_admin checkin_staff hr_admin system_admin],
  ["get", DEBUG_DEMO_CLOCK_PATH] => %w[activity_admin system_admin],
  ["put", DEBUG_DEMO_CLOCK_PATH] => %w[activity_admin system_admin],
  ["get", ADMIN_EVENTS_PATH] => %w[activity_admin checkin_staff hr_admin system_admin],
  ["post", ADMIN_EVENTS_PATH] => %w[activity_admin],
  ["get", "/events"] => %w[employee],
  ["get", "/events/{event_id}"] => %w[employee],
  ["get", EVENT_POSTER_PATH] => %w[employee activity_admin checkin_staff hr_admin system_admin],
  ["get", "/events/{event_id}/eligibility"] => %w[employee],
  ["post", "/events/{event_id}/bookings"] => %w[employee],
  ["get", "/me/tickets"] => %w[employee],
  ["post", "/me/registrations/{registration_id}/cancel"] => %w[employee],
  ["patch", ADMIN_EVENT_PATH] => %w[activity_admin],
  ["delete", ADMIN_EVENT_PATH] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/poster"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/state"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/duplicate"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/eligibility/preview"] => %w[activity_admin],
  ["put", "/admin/events/{event_id}/eligibility"] => %w[activity_admin],
  ["get", "/admin/eligibility-impact-reviews"] => %w[activity_admin hr_admin system_admin],
  ["post", "/admin/eligibility-impact-reviews/{review_id}/resolve"] => %w[activity_admin hr_admin system_admin],
  ["get", "/admin/events/{event_id}/registrations"] => %w[activity_admin hr_admin system_admin],
  ["post", "/admin/events/{event_id}/registrations/{registration_id}/cancel"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/waitlist/promote"] => %w[activity_admin],
  ["delete", "/admin/events/{event_id}/bans/{target_employee_id}"] => %w[activity_admin system_admin],
  ["post", "/admin/events/{event_id}/lottery-runs"] => %w[activity_admin],
  ["get", "/tickets/{ticket_id}"] => %w[employee activity_admin checkin_staff hr_admin system_admin],
  ["post", "/admin/tickets/{ticket_id}/revoke"] => %w[activity_admin hr_admin system_admin],
  ["post", CHECKINS_PATH] => %w[checkin_staff],
  ["get", "/checkins/events/{event_id}/offline-package"] => %w[checkin_staff],
  ["post", "/checkins/offline-sync"] => %w[checkin_staff],
  ["get", NOTIFICATION_PREFERENCES_PATH] => %w[employee],
  ["put", NOTIFICATION_PREFERENCES_PATH] => %w[employee],
  ["get", "/admin/notifications/deliveries"] => %w[activity_admin hr_admin system_admin],
  ["post", "/admin/notifications/deliveries/{delivery_id}/retry"] => %w[activity_admin hr_admin system_admin],
  ["get", "/admin/reports"] => %w[hr_admin system_admin],
  ["post", "/admin/reports/exports"] => %w[hr_admin system_admin],
  ["get", "/admin/reports/exports/{export_id}"] => %w[hr_admin system_admin],
  ["get", REPORT_EXPORT_DOWNLOAD_PATH] => %w[hr_admin system_admin],
  ["get", "/admin/hr/options"] => %w[activity_admin hr_admin system_admin],
  ["get", "/admin/audit-logs"] => %w[hr_admin system_admin],
  ["get", "/admin/ops/capacity-pressure"] => %w[activity_admin hr_admin system_admin],
  ["get", "/admin/ops/queues"] => %w[hr_admin system_admin],
  ["get", "/admin/ops/notification-deliveries"] => %w[hr_admin system_admin],
  ["get", "/admin/ops/report-freshness"] => %w[hr_admin system_admin],
  ["get", "/admin/ops/dashboard"] => %w[activity_admin hr_admin system_admin]
}.freeze

@documents = {}

fail_contract("Missing #{OPENAPI_ROOT}") unless OPENAPI_ROOT.file?

source_files = [OPENAPI_ROOT.to_s] + Dir.glob(OPENAPI_SOURCE_GLOB).sort
source_files.each { |file| load_yaml(file) }
source_files.each do |file|
  each_ref(load_yaml(file)).each { |ref| resolve_ref(ref, file) }
end

root = load_yaml(OPENAPI_ROOT)
paths = root.fetch("paths")
path_names = paths.keys
banned_paths = path_names & BANNED_PATHS
fail_contract("Banned product OpenAPI paths found: #{banned_paths.join(", ")}") unless banned_paths.empty?

security = root.fetch("security", [])
unless security.any? { |entry| entry.is_a?(Hash) && entry.key?("ProviderBearerAuth") }
  fail_contract("Root security must require ProviderBearerAuth")
end

allowed_roles = require_schema_ref(root, "AppRole").fetch("enum")

EXPECTED_OPERATIONS.each do |path, methods|
  fail_contract("Missing OpenAPI path #{path}") unless paths.key?(path)

  item, item_source = path_item_with_source(root, path)
  actual_methods = item.keys.select { |key| HTTP_METHODS.include?(key) }
  missing_methods = methods - actual_methods
  fail_contract("Missing methods for #{path}: #{missing_methods.join(", ")}") unless missing_methods.empty?

  methods.each do |method|
    operation = item.fetch(method)
    operation_id = "#{method.upcase} #{path}"
    public_operation = PUBLIC_OPERATIONS.include?([method, path])
    roles = operation["x-required-roles"]
    if public_operation
      fail_contract("Public operation #{operation_id} must opt out of root security") unless operation["security"] == []
      fail_contract("Public operation #{operation_id} must not declare x-required-roles") if roles
    else
      fail_contract("Missing x-required-roles for #{operation_id}") unless roles.is_a?(Array) && !roles.empty?

      unknown_roles = roles - allowed_roles
      fail_contract("Unknown x-required-roles for #{operation_id}: #{unknown_roles.join(", ")}") unless unknown_roles.empty?

      expected_roles = EXPECTED_REQUIRED_ROLES.fetch([method, path])
      unless roles.sort == expected_roles.sort
        fail_contract("x-required-roles drift for #{operation_id}: got #{roles.join(", ")}, want #{expected_roles.join(", ")}")
      end
    end

    responses = operation.fetch("responses", {})
    fail_contract("Missing 2xx success response for #{operation_id}") unless operation_success_response?(operation)
    unless BINARY_OPERATIONS.include?([method, path]) || operation_json_success_envelope?(operation, item_source)
      fail_contract("Missing JSON success envelope for #{operation_id}")
    end
    unless public_operation
      fail_contract("Missing 401 auth error response for #{operation_id}") unless responses.key?("401")
      fail_contract("Missing 403 authorization error response for #{operation_id}") unless responses.key?("403")
    end
  end
end

unexpected_paths = path_names - EXPECTED_OPERATIONS.keys
fail_contract("Unexpected canonical OpenAPI paths found: #{unexpected_paths.join(", ")}") unless unexpected_paths.empty?

caller_supplied_employee_identity = []
source_files.each do |file|
  walk_hashes(load_yaml(file)) do |hash, path|
    next unless hash["name"] == "employee_id" && %w[path query].include?(hash["in"])

    caller_supplied_employee_identity << "#{Pathname.new(file).relative_path_from(ROOT)} at #{path.join("/")}"
  end
end
unless caller_supplied_employee_identity.empty?
  fail_contract("Caller-supplied employee_id parameters found:\n#{caller_supplied_employee_identity.join("\n")}")
end

capacity_type = require_schema_ref(root, "CapacityType")
unless capacity_type.fetch("enum").sort == %w[limited unlimited]
  fail_contract("CapacityType must enumerate limited and unlimited")
end

ticket_status = require_schema_ref(root, "TicketStatus")
unless ticket_status.fetch("enum").sort == %w[active expired redeemed revoked]
  fail_contract("TicketStatus must match API ticket status values")
end

notification_delivery_status = require_schema_ref(root, "NotificationDeliveryStatus")
expected_delivery_statuses = %w[dead_letter failed pending sending sent suppressed]
unless notification_delivery_status.fetch("enum").sort == expected_delivery_statuses
  fail_contract("NotificationDeliveryStatus must match API notification delivery status values")
end

allocation_mode = require_schema_ref(root, "AllocationMode")
unless allocation_mode.fetch("enum").sort == %w[fcfs lottery]
  fail_contract("AllocationMode must match API allocation mode values")
end

event_summary = require_schema_ref(root, "EventSummary")
require_schema_fields(root, "EventSummary", %w[registration_start registration_close current_user_status rule allocation_mode])
%w[registration_opens_at registration_closes_at current_booking_status eligibility_rule].each do |field|
  fail_contract("EventSummary must not expose stale #{field}") if event_summary.dig("properties", field)
end

event_create = require_schema_ref(root, "CreateEventRequest")
require_schema_fields(root, "CreateEventRequest", %w[registration_start registration_close rule])
%w[registration_opens_at registration_closes_at eligibility_rule].each do |field|
  fail_contract("CreateEventRequest must not expose stale #{field}") if event_create.dig("properties", field)
end

event_update = require_schema_ref(root, "UpdateEventRequest")
%w[registration_start registration_close].each do |field|
  fail_contract("UpdateEventRequest must expose #{field}") unless event_update.dig("properties", field)
end
%w[registration_opens_at registration_closes_at eligibility_rule].each do |field|
  fail_contract("UpdateEventRequest must not expose stale #{field}") if event_update.dig("properties", field)
end

create_booking = require_schema_ref(root, "CreateBookingRequest")
fail_contract("CreateBookingRequest must expose family_count") unless create_booking.dig("properties", "family_count")

offline_scan = require_schema_ref(root, "OfflineCheckInScanInput")
fail_contract("OfflineCheckInScanInput must require scanned_at") unless offline_scan.fetch("required").include?("scanned_at")
if offline_scan.dig("properties", "local_scan_id")
  fail_contract("OfflineCheckInScanInput must not expose unsupported local_scan_id")
end

checkin_error = require_schema_ref(root, "CheckInErrorResponse")
unless checkin_error.dig("properties", "success", "const") == false
  fail_contract("CheckInErrorResponse must be an error envelope")
end
fail_contract("CheckInErrorResponse must expose response data") unless checkin_error.dig("properties", "data")
fail_contract("CheckInErrorResponse must require data") unless checkin_error.fetch("required", []).include?("data")
unless checkin_error.dig("properties", "error", "type") == "string"
  fail_contract("CheckInErrorResponse must expose string error")
end
checkin_operation = path_item_for(root, CHECKINS_PATH).fetch("post")
checkin_success_description = checkin_operation.dig("responses", "200", "description").to_s.downcase
%w[duplicated rejected conflicted].each do |stale_status|
  if checkin_success_description.include?(stale_status)
    fail_contract("POST /checkins 200 response must not describe #{stale_status} outcomes")
  end
end
checkin_conflict_schema = checkin_operation.dig("responses", "409", "content", "application/json", "schema") || {}
unless checkin_conflict_schema["$ref"].to_s.end_with?("CheckInErrorResponse")
  fail_contract("POST /checkins 409 response must use CheckInErrorResponse")
end

require_schema_fields(root, "EmployeeClaims", %w[grade employment_status])
require_schema_fields(root, "MockProfile", %w[grade employment_status])
require_schema_fields(root, "AuthBootstrap", %w[debug_chrome_enabled])
require_schema_fields(root, "NotificationPreferences", %w[employee_id])
notification_delivery = require_schema_ref(root, "NotificationDelivery")
delivery_id_schema = notification_delivery.dig("properties", "delivery_id") || {}
if delivery_id_schema["format"] == "uuid" || delivery_id_schema["pattern"] != "^del_[a-f0-9]{32}$"
  fail_contract("NotificationDelivery.delivery_id must use the del_ prefixed ID pattern")
end
outbox_id_schema = notification_delivery.dig("properties", "outbox_id") || {}
if outbox_id_schema["format"] == "uuid" || outbox_id_schema["pattern"] != "^out_[a-f0-9]{32}$"
  fail_contract("NotificationDelivery.outbox_id must use the out_ prefixed ID pattern")
end
delivery_id_parameter = resolve_if_ref(root.dig("components", "parameters", "DeliveryId"), OPENAPI_ROOT.to_s)
delivery_id_parameter_schema = delivery_id_parameter.fetch("schema")
if delivery_id_parameter_schema["format"] == "uuid" || delivery_id_parameter_schema["pattern"] != "^del_[a-f0-9]{32}$"
  fail_contract("DeliveryId parameter must use the del_ prefixed ID pattern")
end
impact_review = require_schema_ref(root, "EligibilityImpactReview")
fail_contract("EligibilityImpactReview must expose employee_ref") unless impact_review.dig("properties", "employee_ref")
fail_contract("EligibilityImpactReview must require employee_ref") unless impact_review.fetch("required", []).include?("employee_ref")
fail_contract("EligibilityImpactReview must not expose employee_id") if impact_review.dig("properties", "employee_id")
unless impact_review.dig("properties", "status", "enum").sort == %w[pending resolved]
  fail_contract("EligibilityImpactReview status enum must match API values")
end
audit_log = require_schema_ref(root, "AuditLogEntry")
fail_contract("AuditLogEntry must expose metadata") unless audit_log.dig("properties", "metadata", "type") == "string"
fail_contract("AuditLogEntry must require metadata") unless audit_log.fetch("required", []).include?("metadata")
fail_contract("AuditLogEntry must not expose redacted_metadata") if audit_log.dig("properties", "redacted_metadata")

warning = require_schema_ref(root, "Warning")
fail_contract("Warning must include cross_city") unless warning.dig("properties", "code", "enum").include?("cross_city")

no_show_cooldown = require_schema_ref(root, "NoShowCooldown")
fail_contract("NoShowCooldown must expose active state") unless no_show_cooldown.dig("properties", "active")
unless no_show_cooldown.dig("properties", "applies_to", "enum").include?("limited")
  fail_contract("NoShowCooldown must describe limited-event cooldown scope")
end

ticket_schemas = load_yaml(ROOT.join("docs/openapi/components/schemas/tickets.yaml"))
ticket_base = ticket_schemas.fetch("TicketBase")
unless ticket_base.dig("properties", "non_transferable", "const") == true
  fail_contract("Ticket must be explicitly non-transferable")
end

fail_contract("CreateEventRequest must expose capacity_type") unless event_create.dig("properties", "capacity_type")
fail_contract("CreateEventRequest must expose allows_family") unless event_create.dig("properties", "allows_family")
fail_contract("CreateEventRequest must expose allocation_mode") unless event_create.dig("properties", "allocation_mode")

freshness_meta = require_schema_ref(root, "FreshnessMeta")
required_meta_fields = freshness_meta.fetch("required", [])
fail_contract("FreshnessMeta must require as_of") unless required_meta_fields.include?("as_of")
fail_contract("FreshnessMeta must require source") unless required_meta_fields.include?("source")
%w[as_of source lag_seconds degraded].each do |property|
  fail_contract("FreshnessMeta must expose #{property}") unless freshness_meta.dig("properties", property)
end
source_enum = freshness_meta.dig("properties", "source", "enum") || []
%w[reporting_projection derived operational].each do |value|
  fail_contract("FreshnessMeta.source enum must include #{value}") unless source_enum.include?(value)
end

meta_envelope = require_schema_ref(root, "ApiSuccessWithMetaEnvelope")
meta_envelope_parts = meta_envelope.fetch("allOf", [])
unless meta_envelope_parts.any? { |part| part.is_a?(Hash) && part["$ref"].to_s.include?("ApiSuccessEnvelope") }
  fail_contract("ApiSuccessWithMetaEnvelope must compose ApiSuccessEnvelope via allOf")
end
meta_property_part = meta_envelope_parts.find { |part| part.is_a?(Hash) && part.dig("properties", "meta") }
fail_contract("ApiSuccessWithMetaEnvelope must expose a meta property") unless meta_property_part
if (meta_property_part["required"] || []).include?("meta")
  fail_contract("ApiSuccessWithMetaEnvelope.meta must remain optional to preserve Phase 1 backward compatibility")
end

worker_kind = require_schema_ref(root, "WorkerKind")
required_kinds = %w[notification projection compensation export]
missing_kinds = required_kinds - (worker_kind["enum"] || [])
fail_contract("WorkerKind enum must include #{missing_kinds.join(", ")}") unless missing_kinds.empty?

# PH2-05: pin Phase 1 envelope signatures so silent drift can't break Phase 1 clients.
success_envelope = require_schema_ref(root, "ApiSuccessEnvelope")
unless (success_envelope["required"] || []).sort == %w[data error success]
  fail_contract("ApiSuccessEnvelope.required must remain [success, data, error]")
end
unless success_envelope.dig("properties", "success", "const") == true
  fail_contract("ApiSuccessEnvelope.properties.success.const must remain true")
end
unless success_envelope.dig("properties", "error", "type") == "null"
  fail_contract("ApiSuccessEnvelope.properties.error.type must remain \"null\"")
end

error_envelope = require_schema_ref(root, "ApiErrorEnvelope")
unless (error_envelope["required"] || []).sort == %w[data error success]
  fail_contract("ApiErrorEnvelope.required must remain [success, data, error]")
end
unless error_envelope.dig("properties", "success", "const") == false
  fail_contract("ApiErrorEnvelope.properties.success.const must remain false")
end
unless error_envelope.dig("properties", "data", "type") == "null"
  fail_contract("ApiErrorEnvelope.properties.data.type must remain \"null\"")
end

envelope_union = require_schema_ref(root, "ApiEnvelope")
union_refs = (envelope_union["oneOf"] || []).map { |part| part.is_a?(Hash) ? part["$ref"].to_s : "" }
%w[ApiSuccessEnvelope ApiErrorEnvelope].each do |name|
  unless union_refs.any? { |ref| ref.include?(name) }
    fail_contract("ApiEnvelope.oneOf must continue to reference #{name}")
  end
end

warning_schema = require_schema_ref(root, "Warning")
phase1_warning_codes = %w[cross_city registration_closing_soon non_transferable_ticket]
warning_enum = warning_schema.dig("properties", "code", "enum") || []
missing_codes = phase1_warning_codes - warning_enum
unless missing_codes.empty?
  fail_contract("Warning.code enum must continue to expose Phase 1 codes #{missing_codes.join(", ")}")
end

# PH2-05: scan all description strings for forbidden Phase 2 completion language.
PHASE2_FORBIDDEN_PHRASES = [
  "phase 2 requires kafka", "phase 2 uses kafka", "phase 2 has kafka",
  "phase 2 implements kafka", "phase 2 ships kafka", "kafka is complete in phase 2",
  "phase 2 completed kafka", "kafka completed in phase 2",
  "phase 2 requires kubernetes", "phase 2 uses kubernetes", "phase 2 has kubernetes",
  "phase 2 implements kubernetes", "phase 2 ships kubernetes", "kubernetes is complete in phase 2",
  "phase 2 completed kubernetes", "kubernetes completed in phase 2",
  "phase 2 requires service mesh", "phase 2 uses service mesh", "phase 2 has service mesh",
  "phase 2 implements service mesh", "phase 2 ships service mesh", "service mesh is complete in phase 2",
  "phase 2 completed service mesh", "service mesh completed in phase 2",
  "phase 2 requires cross-region ha", "phase 2 has cross-region ha",
  "phase 2 has cross-region high availability", "phase 2 implements cross-region ha",
  "phase 2 ships cross-region ha", "cross-region ha is complete in phase 2",
  "phase 2 completed cross-region ha",
  "phase 2 implements microservices", "phase 2 has microservices",
  "phase 2 has full microservices", "phase 2 implements full microservices",
  "phase 2 ships microservices", "phase 2 ships full microservices",
  "phase 2 completed microservices", "microservices are complete in phase 2",
].freeze

description_offenders = []
walk = lambda do |node, trail|
  case node
  when Hash
    node.each do |key, value|
      if key == "description" && value.is_a?(String)
        lower = value.downcase
        PHASE2_FORBIDDEN_PHRASES.each do |phrase|
          description_offenders << "#{trail.join("/")}: #{phrase}" if lower.include?(phrase)
        end
      else
        walk.call(value, trail + [key.to_s])
      end
    end
  when Array
    node.each_with_index { |item, idx| walk.call(item, trail + ["[#{idx}]"]) }
  else
    nil
  end
end
walk.call(root, [])
unless description_offenders.empty?
  fail_contract("OpenAPI description strings contain forbidden Phase 2 language:\n  - " + description_offenders.uniq.join("\n  - "))
end

puts "openapi source files parsed: #{source_files.length}"
puts "openapi paths verified: #{EXPECTED_OPERATIONS.length}"
puts "openapi refs resolved"
puts "openapi role metadata passed"
puts "openapi response envelope coverage passed"
puts "openapi product auth boundary passed"
puts "openapi COR-18 rule coverage passed"
puts "openapi PH2-02 freshness meta + worker kind coverage passed"
puts "openapi PH2-05 Phase 1 envelope drift + Phase 2 language guard passed"
