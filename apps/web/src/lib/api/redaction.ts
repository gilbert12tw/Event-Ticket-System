export function redact(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(redact);
  if (!value || typeof value !== "object") return value;
  const output: Record<string, unknown> = {};
  for (const [key, raw] of Object.entries(value)) {
    if (key === "provider_token") {
      output[key] =
        typeof raw === "string" && raw.length > 0
          ? "[redacted provider token]"
          : raw;
    } else if (key === "signed_token" || key === "qr_payload") {
      output[key] =
        typeof raw === "string" && raw.length > 0
          ? "[redacted ticket token]"
          : raw;
    } else if (key === "cets_session" || key === "session" || key === "token") {
      output[key] =
        typeof raw === "string" && raw.length > 0 ? "[redacted session]" : raw;
    } else {
      output[key] = redact(raw);
    }
  }
  return output;
}
