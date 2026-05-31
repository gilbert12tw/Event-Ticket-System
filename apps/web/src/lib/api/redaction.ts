const providerTokenKeys = new Set(["provider_token"]);
const ticketSignatureKeys = new Set(["signed_token", "qr_payload", "qr_token"]);
const ticketHashKeys = new Set([
  "package_signature",
  "token_hash",
  "signed_token_hash",
]);
const sessionTokenKeys = new Set(["cets_session", "session", "token"]);

export function redact(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(redact);
  if (!value || typeof value !== "object") return value;
  const output: Record<string, unknown> = {};
  for (const [key, raw] of Object.entries(value)) {
    if (providerTokenKeys.has(key)) {
      output[key] = redactedValue(raw, "身分簽章");
    } else if (ticketSignatureKeys.has(key)) {
      output[key] = redactedValue(raw, "票券簽章");
    } else if (ticketHashKeys.has(key)) {
      output[key] = redactedValue(raw, "票券證據", true);
    } else if (sessionTokenKeys.has(key)) {
      output[key] = redactedValue(raw, "工作階段");
    } else {
      output[key] = redact(raw);
    }
  }
  return output;
}

export function fingerprint(raw: string) {
  let hash = 0x811c9dc5;
  for (const char of raw) {
    hash ^= char.codePointAt(0) || 0;
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(16).padStart(8, "0");
}

function redactedValue(
  raw: unknown,
  label: string,
  includeFingerprint = false,
) {
  if (typeof raw !== "string" || raw.length === 0) return raw;
  const suffix = includeFingerprint ? ` #${fingerprint(raw)}` : "";
  return `[${label}已遮蔽${suffix}]`;
}
