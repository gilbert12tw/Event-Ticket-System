# frozen_string_literal: true

require "pathname"
require "yaml"

ROOT = Pathname.new(__dir__).join("..").expand_path
OPENAPI_ROOT = ROOT.join("docs/openapi.yaml")
OPENAPI_SOURCE_GLOB = ROOT.join("docs/openapi/**/*.yaml").to_s

HTTP_METHODS = %w[get put post delete options head patch trace].freeze
BANNED_PATHS = [
  "/auth/login",
  "/auth/logout",
  "/employees/{employee_id}/tickets"
].freeze

EXPECTED_OPERATIONS = {
  "/auth/me" => %w[get],
  "/admin/events" => %w[get post],
  "/events" => %w[get],
  "/events/{event_id}" => %w[get],
  "/events/{event_id}/eligibility" => %w[get],
  "/events/{event_id}/bookings" => %w[post],
  "/me/tickets" => %w[get],
  "/me/registrations/{registration_id}/cancel" => %w[post],
  "/admin/events/{event_id}" => %w[patch delete],
  "/admin/events/{event_id}/state" => %w[post],
  "/admin/events/{event_id}/duplicate" => %w[post],
  "/admin/events/{event_id}/eligibility/preview" => %w[post],
  "/admin/events/{event_id}/eligibility" => %w[put],
  "/admin/eligibility-impact-reviews" => %w[get],
  "/admin/eligibility-impact-reviews/{review_id}/resolve" => %w[post],
  "/admin/events/{event_id}/registrations" => %w[get],
  "/admin/events/{event_id}/registrations/{registration_id}/cancel" => %w[post],
  "/admin/events/{event_id}/waitlist/promote" => %w[post],
  "/admin/events/{event_id}/lottery-runs" => %w[post],
  "/tickets/{ticket_id}" => %w[get],
  "/admin/tickets/{ticket_id}/revoke" => %w[post],
  "/checkins" => %w[post],
  "/checkins/events/{event_id}/offline-package" => %w[get],
  "/checkins/offline-sync" => %w[post],
  "/notifications/preferences" => %w[get put],
  "/admin/notifications/deliveries" => %w[get],
  "/admin/notifications/deliveries/{delivery_id}/retry" => %w[post],
  "/admin/reports" => %w[get],
  "/admin/reports/exports" => %w[post],
  "/admin/reports/exports/{export_id}" => %w[get],
  "/admin/audit-logs" => %w[get]
}.freeze

EXPECTED_REQUIRED_ROLES = {
  ["get", "/auth/me"] => %w[employee activity_admin checkin_staff hr_admin system_admin],
  ["get", "/admin/events"] => %w[activity_admin checkin_staff hr_admin system_admin],
  ["post", "/admin/events"] => %w[activity_admin],
  ["get", "/events"] => %w[employee],
  ["get", "/events/{event_id}"] => %w[employee],
  ["get", "/events/{event_id}/eligibility"] => %w[employee],
  ["post", "/events/{event_id}/bookings"] => %w[employee],
  ["get", "/me/tickets"] => %w[employee],
  ["post", "/me/registrations/{registration_id}/cancel"] => %w[employee],
  ["patch", "/admin/events/{event_id}"] => %w[activity_admin],
  ["delete", "/admin/events/{event_id}"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/state"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/duplicate"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/eligibility/preview"] => %w[activity_admin],
  ["put", "/admin/events/{event_id}/eligibility"] => %w[activity_admin],
  ["get", "/admin/eligibility-impact-reviews"] => %w[activity_admin hr_admin system_admin],
  ["post", "/admin/eligibility-impact-reviews/{review_id}/resolve"] => %w[activity_admin hr_admin system_admin],
  ["get", "/admin/events/{event_id}/registrations"] => %w[activity_admin hr_admin system_admin],
  ["post", "/admin/events/{event_id}/registrations/{registration_id}/cancel"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/waitlist/promote"] => %w[activity_admin],
  ["post", "/admin/events/{event_id}/lottery-runs"] => %w[activity_admin],
  ["get", "/tickets/{ticket_id}"] => %w[employee activity_admin checkin_staff hr_admin system_admin],
  ["post", "/admin/tickets/{ticket_id}/revoke"] => %w[activity_admin hr_admin system_admin],
  ["post", "/checkins"] => %w[checkin_staff],
  ["get", "/checkins/events/{event_id}/offline-package"] => %w[checkin_staff],
  ["post", "/checkins/offline-sync"] => %w[checkin_staff],
  ["get", "/notifications/preferences"] => %w[employee],
  ["put", "/notifications/preferences"] => %w[employee],
  ["get", "/admin/notifications/deliveries"] => %w[activity_admin hr_admin system_admin],
  ["post", "/admin/notifications/deliveries/{delivery_id}/retry"] => %w[activity_admin hr_admin system_admin],
  ["get", "/admin/reports"] => %w[hr_admin system_admin],
  ["post", "/admin/reports/exports"] => %w[hr_admin system_admin],
  ["get", "/admin/reports/exports/{export_id}"] => %w[hr_admin system_admin],
  ["get", "/admin/audit-logs"] => %w[hr_admin system_admin]
}.freeze

@documents = {}

def fail_contract(message)
  warn "OpenAPI contract check failed: #{message}"
  exit 1
end

def load_yaml(path)
  expanded = Pathname.new(path).expand_path
  @documents[expanded.to_s] ||= YAML.load_file(expanded.to_s) || {}
rescue Psych::SyntaxError => e
  fail_contract("Invalid YAML in #{expanded}: #{e.message}")
end

def pointer_token(token)
  token.gsub("~1", "/").gsub("~0", "~")
end

def resolve_pointer(document, fragment, source)
  return document if fragment.nil? || fragment.empty?

  fail_contract("Unsupported $ref fragment ##{fragment} in #{source}") unless fragment.start_with?("/")

  fragment.split("/").drop(1).reduce(document) do |current, raw_token|
    token = pointer_token(raw_token)
    next current[token] if current.is_a?(Hash) && current.key?(token)
    next current[token.to_i] if current.is_a?(Array) && token.match?(/\A\d+\z/) && current.length > token.to_i

    fail_contract("Unresolved $ref fragment ##{fragment} in #{source}")
  end
end

def resolve_ref(ref, base_file)
  file_part, fragment = ref.split("#", 2)
  target_file =
    if file_part.nil? || file_part.empty?
      Pathname.new(base_file).expand_path
    else
      Pathname.new(base_file).dirname.join(file_part).expand_path
    end

  fail_contract("Missing $ref target file #{target_file} from #{base_file}") unless target_file.file?

  [resolve_pointer(load_yaml(target_file), fragment.to_s, "#{base_file} -> #{ref}"), target_file.to_s]
end

def resolve_if_ref(value, base_file)
  return value unless value.is_a?(Hash) && value["$ref"].is_a?(String)

  resolved, = resolve_ref(value.fetch("$ref"), base_file)
  resolved
end

def each_ref(value, refs = [])
  case value
  when Hash
    refs << value["$ref"] if value["$ref"].is_a?(String)
    value.each_value { |child| each_ref(child, refs) }
  when Array
    value.each { |child| each_ref(child, refs) }
  end
  refs
end

def walk_hashes(value, path = [], &block)
  case value
  when Hash
    yield value, path
    value.each { |key, child| walk_hashes(child, path + [key], &block) }
  when Array
    value.each_with_index { |child, index| walk_hashes(child, path + [index], &block) }
  end
end

def path_item_for(root, path)
  item = root.fetch("paths").fetch(path)
  item = resolve_if_ref(item, OPENAPI_ROOT.to_s)
  item
end

def operation_success_response?(operation)
  operation.fetch("responses", {}).keys.any? { |status| status.to_s.match?(/\A2\d\d\z/) }
end

def operation_json_schema?(operation)
  operation.fetch("responses", {}).any? do |status, response|
    next false unless status.to_s.match?(/\A2\d\d\z/)

    resolved = resolve_if_ref(response, OPENAPI_ROOT.to_s)
    resolved.dig("content", "application/json", "schema").is_a?(Hash)
  end
end

def require_schema_ref(root, name)
  entry = root.dig("components", "schemas", name)
  fail_contract("Missing component schema #{name}") unless entry

  resolve_if_ref(entry, OPENAPI_ROOT.to_s)
end

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

  item = path_item_for(root, path)
  actual_methods = item.keys.select { |key| HTTP_METHODS.include?(key) }
  missing_methods = methods - actual_methods
  fail_contract("Missing methods for #{path}: #{missing_methods.join(", ")}") unless missing_methods.empty?

  methods.each do |method|
    operation = item.fetch(method)
    operation_id = "#{method.upcase} #{path}"
    roles = operation["x-required-roles"]
    fail_contract("Missing x-required-roles for #{operation_id}") unless roles.is_a?(Array) && !roles.empty?

    unknown_roles = roles - allowed_roles
    fail_contract("Unknown x-required-roles for #{operation_id}: #{unknown_roles.join(", ")}") unless unknown_roles.empty?

    expected_roles = EXPECTED_REQUIRED_ROLES.fetch([method, path])
    unless roles.sort == expected_roles.sort
      fail_contract("x-required-roles drift for #{operation_id}: got #{roles.join(", ")}, want #{expected_roles.join(", ")}")
    end

    responses = operation.fetch("responses", {})
    fail_contract("Missing 2xx success response for #{operation_id}") unless operation_success_response?(operation)
    fail_contract("Missing JSON success envelope for #{operation_id}") unless operation_json_schema?(operation)
    fail_contract("Missing 401 auth error response for #{operation_id}") unless responses.key?("401")
    fail_contract("Missing 403 authorization error response for #{operation_id}") unless responses.key?("403")
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

create_booking = require_schema_ref(root, "CreateBookingRequest")
fail_contract("CreateBookingRequest must expose family_count") unless create_booking.dig("properties", "family_count")

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

event_create = require_schema_ref(root, "CreateEventRequest")
fail_contract("CreateEventRequest must expose capacity_type") unless event_create.dig("properties", "capacity_type")
fail_contract("CreateEventRequest must expose allows_family") unless event_create.dig("properties", "allows_family")
fail_contract("CreateEventRequest must expose allocation_mode") unless event_create.dig("properties", "allocation_mode")

puts "openapi source files parsed: #{source_files.length}"
puts "openapi paths verified: #{EXPECTED_OPERATIONS.length}"
puts "openapi refs resolved"
puts "openapi role metadata passed"
puts "openapi response envelope coverage passed"
puts "openapi product auth boundary passed"
puts "openapi COR-18 rule coverage passed"
