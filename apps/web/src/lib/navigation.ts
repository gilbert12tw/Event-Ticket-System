import type { MouseEvent } from "react";

export function shouldUseNativeNavigation(
  event: MouseEvent<HTMLAnchorElement>,
) {
  return (
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    event.shiftKey
  );
}

export function runClientNavigation(
  event: MouseEvent<HTMLAnchorElement>,
  action: () => void,
) {
  if (shouldUseNativeNavigation(event)) return false;
  event.preventDefault();
  action();
  return true;
}
