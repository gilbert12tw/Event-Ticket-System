import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { registerServiceWorker } from "./register-sw";

const descriptor = Object.getOwnPropertyDescriptor(navigator, "serviceWorker");

function setServiceWorker(value: unknown) {
  Object.defineProperty(navigator, "serviceWorker", {
    value,
    configurable: true,
  });
}

// registerServiceWorker attaches an anonymous window "load" listener with no
// cleanup handle. Spy on addEventListener (which calls through by default) so we
// can detach whatever it registered after each test, preventing stale callbacks
// from firing on a later test's dispatch.
let addEventListenerSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  addEventListenerSpy = vi.spyOn(window, "addEventListener");
});

afterEach(() => {
  for (const [type, listener] of addEventListenerSpy.mock.calls) {
    if (type === "load" && listener) {
      window.removeEventListener("load", listener);
    }
  }
  addEventListenerSpy.mockRestore();
  if (descriptor) {
    Object.defineProperty(navigator, "serviceWorker", descriptor);
  } else {
    setServiceWorker(undefined);
  }
});

describe("registerServiceWorker", () => {
  it("does nothing when service workers are unsupported", () => {
    // Remove the property entirely so the `in` check is false.
    delete (navigator as { serviceWorker?: unknown }).serviceWorker;
    expect(() => registerServiceWorker()).not.toThrow();
  });

  it("registers the worker once the window load event fires", async () => {
    const register = vi.fn().mockResolvedValue(undefined);
    setServiceWorker({ register });

    registerServiceWorker();
    window.dispatchEvent(new Event("load"));

    expect(register).toHaveBeenCalledWith("/sw.js");
  });

  it("swallows a failed registration", async () => {
    const register = vi.fn().mockRejectedValue(new Error("nope"));
    setServiceWorker({ register });

    registerServiceWorker();
    window.dispatchEvent(new Event("load"));

    await Promise.resolve();
    expect(register).toHaveBeenCalled();
  });
});
