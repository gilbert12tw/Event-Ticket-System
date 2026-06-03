# frozen_string_literal: true

require "pathname"

SUCCESS_ENVELOPE_SCHEMAS = %w[
  ApiSuccessEnvelope
  ApiSuccessWithMetaEnvelope
].freeze

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

def resolve_if_ref_with_source(value, base_file)
  return [value, base_file] unless value.is_a?(Hash) && value["$ref"].is_a?(String)

  resolve_ref(value.fetch("$ref"), base_file)
end

def each_ref(value, refs = [])
  case value
  when Hash
    refs << value["$ref"] if value["$ref"].is_a?(String)
    value.each_value { |child| each_ref(child, refs) }
  when Array
    value.each { |child| each_ref(child, refs) }
  else
    nil
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
  else
    nil
  end
end

def path_item_for(root, path)
  item, = path_item_with_source(root, path)
  item
end

def path_item_with_source(root, path)
  item = root.fetch("paths").fetch(path)
  return resolve_ref(item.fetch("$ref"), OPENAPI_ROOT.to_s) if item.is_a?(Hash) && item["$ref"].is_a?(String)

  [item, OPENAPI_ROOT.to_s]
end

def operation_success_response?(operation)
  operation.fetch("responses", {}).keys.any? { |status| status.to_s.match?(/\A2\d\d\z/) }
end

def success_envelope_ref?(ref)
  _, fragment = ref.split("#", 2)
  schema_name = pointer_token(fragment.to_s.split("/").last.to_s)
  SUCCESS_ENVELOPE_SCHEMAS.include?(schema_name)
end

def schema_references_success_envelope?(schema, base_file, seen = {})
  case schema
  when Hash
    ref = schema["$ref"]
    if ref.is_a?(String)
      return true if success_envelope_ref?(ref)

      resolved, target_file = resolve_ref(ref, base_file)
      seen_key = "#{target_file}##{ref.split("#", 2).last}"
      return false if seen[seen_key]

      seen[seen_key] = true
      return schema_references_success_envelope?(resolved, target_file, seen)
    end

    schema.any? { |_, child| schema_references_success_envelope?(child, base_file, seen) }
  when Array
    schema.any? { |child| schema_references_success_envelope?(child, base_file, seen) }
  else
    false
  end
end

def operation_json_success_envelope?(operation, base_file)
  success_responses = operation.fetch("responses", {}).select do |status, _response|
    status.to_s.match?(/\A2\d\d\z/)
  end
  return false if success_responses.empty?

  success_responses.all? do |_status, response|
    resolved, response_source = resolve_if_ref_with_source(response, base_file)
    schema = resolved.dig("content", "application/json", "schema")
    schema.is_a?(Hash) && schema_references_success_envelope?(schema, response_source)
  end
end

def require_schema_ref(root, name)
  entry = root.dig("components", "schemas", name)
  fail_contract("Missing component schema #{name}") unless entry

  resolve_if_ref(entry, OPENAPI_ROOT.to_s)
end

def require_schema_fields(root, schema_name, fields)
  schema = require_schema_ref(root, schema_name)
  fields.each do |field|
    fail_contract("#{schema_name} must expose #{field}") unless schema.dig("properties", field)
    fail_contract("#{schema_name} must require #{field}") unless schema.fetch("required", []).include?(field)
  end
end
