import { useCallback, useEffect, useState } from "react";

export function useUrlTab<T extends string>(
  param: string,
  allowed: readonly T[],
  fallback: T,
) {
  const [value, setValue] = useState<T>(() =>
    readTab(param, allowed, fallback),
  );

  useEffect(() => {
    const onPopState = () => setValue(readTab(param, allowed, fallback));
    globalThis.addEventListener("popstate", onPopState);
    return () => globalThis.removeEventListener("popstate", onPopState);
  }, [allowed, fallback, param]);

  const update = useCallback(
    (next: T) => {
      setValue(next);
      const url = new URL(globalThis.location.href);
      url.searchParams.set(param, next);
      globalThis.history.replaceState(globalThis.history.state ?? {}, "", url);
    },
    [param],
  );

  return [value, update] as const;
}

function readTab<T extends string>(
  param: string,
  allowed: readonly T[],
  fallback: T,
) {
  const current = new URLSearchParams(globalThis.location.search).get(param);
  return allowed.includes(current as T) ? (current as T) : fallback;
}
