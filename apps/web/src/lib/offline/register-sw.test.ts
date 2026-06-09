import { afterEach, describe, expect, it, vi } from "vitest";
import { registerServiceWorker } from "./register-sw";

const descriptor = Object.getOwnPropertyDescriptor(navigator, "serviceWorker");

function setServiceWorker(value: unknown) {
  Object.defineProperty(navigator, "serviceWorker", {
    value,
    configurable: true,
  });
}

afterEach(() => {
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
