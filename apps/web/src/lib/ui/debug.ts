const debugParam = "debug";

export function isDebugChromeEnabled() {
  if (!import.meta.env.DEV) return false;
  return new URLSearchParams(window.location.search).get(debugParam) === "1";
}

export function isDebugChromeAvailable() {
  return import.meta.env.DEV;
}

export function debugChromePath(path: string) {
  if (!isDebugChromeEnabled()) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set(debugParam, "1");
  return `${url.pathname}${url.search}${url.hash}`;
}

export function setDebugChromeQuery(enabled: boolean) {
  const params = new URLSearchParams(window.location.search);
  if (enabled) {
    params.set(debugParam, "1");
  } else {
    params.delete(debugParam);
  }
  const query = params.toString();
  window.history.replaceState(
    {},
    "",
    `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`,
  );
}
