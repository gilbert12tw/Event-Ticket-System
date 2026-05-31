const debugParam = "debug";

export function debugChromeDefaultEnabled() {
  return import.meta.env.DEV;
}

export function isDebugChromeEnabled(
  available = true,
  defaultEnabled = debugChromeDefaultEnabled(),
) {
  if (!available) return false;
  const value = new URLSearchParams(globalThis.location.search).get(debugParam);
  if (value === "1") return true;
  if (value === "0") return false;
  return defaultEnabled;
}

export function debugChromePath(path: string) {
  const currentValue = new URLSearchParams(globalThis.location.search).get(
    debugParam,
  );
  if (!currentValue) return path;
  const url = new URL(path, globalThis.location.origin);
  url.searchParams.set(debugParam, currentValue);
  return `${url.pathname}${url.search}${url.hash}`;
}

export function setDebugChromeQuery(
  enabled: boolean,
  defaultEnabled = debugChromeDefaultEnabled(),
) {
  const params = new URLSearchParams(globalThis.location.search);
  if (enabled === defaultEnabled) {
    params.delete(debugParam);
  } else {
    params.set(debugParam, enabled ? "1" : "0");
  }
  const query = params.toString();
  const querySuffix = query ? `?${query}` : "";
  globalThis.history.replaceState(
    {},
    "",
    `${globalThis.location.pathname}${querySuffix}${globalThis.location.hash}`,
  );
}
