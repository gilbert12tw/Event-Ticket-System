import { useCallback, useEffect, useState } from "react";

// Per-employee list of calendar events the user chose to hide. Hidden events
// are dropped from the calendar view to reduce clutter, but stay discoverable
// in the event list (flagged with a "已隱藏" badge). Stored client-side only;
// this is a personal view preference, not booking state.
const STORAGE_PREFIX = "cets-hidden-events";

function storageKey(principalID: string) {
  return `${STORAGE_PREFIX}:${principalID}`;
}

export function loadHiddenEventIds(principalID: string): string[] {
  if (!principalID) return [];
  try {
    const raw = localStorage.getItem(storageKey(principalID));
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((id): id is string => typeof id === "string");
  } catch {
    return [];
  }
}

export function saveHiddenEventIds(principalID: string, ids: string[]): void {
  if (!principalID) return;
  try {
    localStorage.setItem(storageKey(principalID), JSON.stringify(ids));
  } catch {
    /* quota exceeded or private mode — skip silently */
  }
}

export function useHiddenEvents(principalID: string) {
  const [hiddenIds, setHiddenIds] = useState<ReadonlySet<string>>(
    () => new Set(loadHiddenEventIds(principalID)),
  );

  useEffect(() => {
    setHiddenIds(new Set(loadHiddenEventIds(principalID)));
  }, [principalID]);

  const setHidden = useCallback(
    (eventID: string, hidden: boolean) => {
      setHiddenIds((current) => {
        if (hidden === current.has(eventID)) return current;
        const next = new Set(current);
        if (hidden) next.add(eventID);
        else next.delete(eventID);
        saveHiddenEventIds(principalID, Array.from(next));
        return next;
      });
    },
    [principalID],
  );

  return { hiddenIds, setHidden };
}
