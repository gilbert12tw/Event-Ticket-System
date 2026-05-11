# frozen_string_literal: true

require "fileutils"
require "pathname"
require "set"
require "yaml"

ROOT = Pathname.new(__dir__).join("..").expand_path
OPENAPI_ROOT = ROOT.join("docs/openapi.yaml")
OPENAPI_COMPONENTS = ROOT.join("docs/openapi/components")
OPENAPI_BUNDLE = ROOT.join("apps/web/public/openapi/openapi.bundle.yaml")
BACKEND_OPENAPI_BUNDLE = ROOT.join("services/api/internal/httpapi/static/openapi/openapi.bundle.yaml")
SWAGGER_UI_SOURCE = ROOT.join("apps/web/node_modules/swagger-ui-dist")
SWAGGER_UI_PUBLIC = ROOT.join("apps/web/public/swagger-ui")
SWAGGER_UI_ASSETS = %w[
  swagger-ui-bundle.js
  swagger-ui-standalone-preset.js
  swagger-ui.css
].freeze

@documents = {}
@components = {
  "securitySchemes" => {},
  "parameters" => {},
  "schemas" => {},
  "responses" => {}
}
@registering = Set.new

def load_yaml(path)
  expanded = Pathname.new(path).expand_path
  @documents[expanded.to_s] ||= YAML.load_file(expanded.to_s) || {}
end

def deep_copy(value)
  Marshal.load(Marshal.dump(value))
end

def pointer_token(token)
  token.gsub("~1", "/").gsub("~0", "~")
end

def resolve_pointer(document, fragment, source)
  return document if fragment.nil? || fragment.empty?

  unless fragment.start_with?("/")
    abort("Unsupported $ref fragment ##{fragment} in #{source}")
  end

  fragment.split("/").drop(1).reduce(document) do |current, raw_token|
    token = pointer_token(raw_token)
    next current[token] if current.is_a?(Hash) && current.key?(token)
    next current[token.to_i] if current.is_a?(Array) && token.match?(/\A\d+\z/) && current.length > token.to_i

    abort("Unresolved $ref fragment ##{fragment} in #{source}")
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

  abort("Missing $ref target file #{target_file} from #{base_file}") unless target_file.file?

  [resolve_pointer(load_yaml(target_file), fragment.to_s, "#{base_file} -> #{ref}"), target_file.to_s, fragment.to_s]
end

def component_section_for(file)
  path = Pathname.new(file).expand_path
  relative = path.relative_path_from(OPENAPI_COMPONENTS).to_s

  case relative
  when "security-schemes.yaml"
    "securitySchemes"
  when "parameters.yaml"
    "parameters"
  when "responses.yaml"
    "responses"
  when %r{\Aschemas/[^/]+\.yaml\z}
    "schemas"
  end
rescue ArgumentError
  nil
end

def component_name(fragment)
  tokens = fragment.to_s.split("/").reject(&:empty?).map { |token| pointer_token(token) }
  tokens.last
end

def register_component(target_file, fragment)
  section = component_section_for(target_file)
  name = component_name(fragment)
  return nil unless section && name

  key = "#{section}/#{name}"
  return "#/components/#{section}/#{name}" if @components.fetch(section).key?(name)
  return "#/components/#{section}/#{name}" if @registering.include?(key)

  @registering.add(key)
  raw = resolve_pointer(load_yaml(target_file), fragment, target_file)
  @components.fetch(section)[name] = transform_refs(deep_copy(raw), target_file)
  @registering.delete(key)

  "#/components/#{section}/#{name}"
end

def transform_ref(ref, base_file)
  _target, target_file, fragment = resolve_ref(ref, base_file)
  internal_ref = register_component(target_file, fragment)
  return { "$ref" => internal_ref } if internal_ref

  resolved, resolved_file, = resolve_ref(ref, base_file)
  transform_refs(deep_copy(resolved), resolved_file)
end

def transform_refs(value, base_file)
  case value
  when Hash
    return transform_ref(value["$ref"], base_file) if value["$ref"].is_a?(String) && value.keys == ["$ref"]

    value.each_with_object({}) do |(key, child), transformed|
      transformed[key] =
        if key == "$ref" && child.is_a?(String)
          transform_ref(child, base_file).fetch("$ref")
        else
          transform_refs(child, base_file)
        end
    end
  when Array
    value.map { |child| transform_refs(child, base_file) }
  else
    value
  end
end

def write_yaml(path, document)
  content = YAML.dump(document).lines.map { |line| "#{line.rstrip}\n" }.join
  File.write(path, content)
end

root = load_yaml(OPENAPI_ROOT)
bundle = root.reject { |key, _| %w[paths components].include?(key) }

bundle["paths"] = root.fetch("paths").each_with_object({}) do |(path, item), paths|
  resolved, resolved_file, = item.is_a?(Hash) && item["$ref"] ? resolve_ref(item["$ref"], OPENAPI_ROOT.to_s) : [item, OPENAPI_ROOT.to_s]
  paths[path] = transform_refs(deep_copy(resolved), resolved_file)
end

root.fetch("components", {}).each do |_section, entries|
  entries.each_value { |entry| transform_refs(deep_copy(entry), OPENAPI_ROOT.to_s) }
end

bundle["components"] = @components.reject { |_section, entries| entries.empty? }

FileUtils.mkdir_p(OPENAPI_BUNDLE.dirname)
write_yaml(OPENAPI_BUNDLE, bundle)

puts "Bundled #{OPENAPI_ROOT.relative_path_from(ROOT)} -> #{OPENAPI_BUNDLE.relative_path_from(ROOT)}"

FileUtils.mkdir_p(BACKEND_OPENAPI_BUNDLE.dirname)
write_yaml(BACKEND_OPENAPI_BUNDLE, bundle)

puts "Bundled #{OPENAPI_ROOT.relative_path_from(ROOT)} -> #{BACKEND_OPENAPI_BUNDLE.relative_path_from(ROOT)}"

FileUtils.mkdir_p(SWAGGER_UI_PUBLIC)
SWAGGER_UI_ASSETS.each do |asset|
  source = SWAGGER_UI_SOURCE.join(asset)
  abort("Missing Swagger UI asset #{source}; run pnpm install") unless source.file?

  FileUtils.cp(source, SWAGGER_UI_PUBLIC.join(asset))
end

puts "Copied Swagger UI assets -> #{SWAGGER_UI_PUBLIC.relative_path_from(ROOT)}"
