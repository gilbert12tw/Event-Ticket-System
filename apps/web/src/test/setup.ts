import { webcrypto } from "node:crypto";
import "@testing-library/jest-dom/vitest";

if (!globalThis.crypto?.subtle) {
  Object.defineProperty(globalThis, "crypto", { value: webcrypto });
}
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// Coverage instrumentation slows renders enough that the 1000ms default for
// findBy/waitFor occasionally times out. Raise it well below the Vitest test
// timeout so async queries stay stable under load without masking real hangs.
configure({ asyncUtilTimeout: 5000 });

if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false;
}

if (!Element.prototype.setPointerCapture) {
  Element.prototype.setPointerCapture = () => undefined;
}

if (!Element.prototype.releasePointerCapture) {
  Element.prototype.releasePointerCapture = () => undefined;
}

if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => undefined;
}

if (!globalThis.localStorage) {
  const storage = new Map<string, string>();
  globalThis.localStorage = {
    get length() {
      return storage.size;
    },
    clear: () => storage.clear(),
    getItem: (key: string) => storage.get(key) ?? null,
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    removeItem: (key: string) => {
      storage.delete(key);
    },
    setItem: (key: string, value: string) => {
      storage.set(key, String(value));
    },
  };
}

afterEach(() => {
  cleanup();
});
