import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Ticket } from "@/lib/api";

// In-memory IndexedDB stand-in so the store logic runs without jsdom IDB.
const store = new Map<string, unknown>();

vi.mock("./db", () => ({
  clearStore: vi.fn(async () => {
    store.clear();
  }),
  putItem: vi.fn(async (_name: string, item: { ticket_id: string }) => {
    store.set(item.ticket_id, structuredClone(item));
  }),
  getAllItems: vi.fn(async () =>
    Array.from(store.values()).map((v) => structuredClone(v)),
  ),
  getItem: vi.fn(async (_name: string, key: string) =>
    store.has(key) ? structuredClone(store.get(key)) : undefined,
  ),
}));

import {
  cacheTickets,
  loadCachedTicket,
  loadCachedTickets,
} from "./tickets-store";

function ticket(id: string): Ticket {
  return { ticket_id: id, event_id: "evt-1" } as Ticket;
}

beforeEach(() => {
  store.clear();
});

describe("tickets-store", () => {
  it("caches tickets after clearing the previous batch", async () => {
    store.set("stale", ticket("stale"));
    await cacheTickets([ticket("t-1"), ticket("t-2")]);

    const all = await loadCachedTickets();
    expect(all.map((t) => t.ticket_id).sort()).toEqual(["t-1", "t-2"]);
  });

  it("loads a single cached ticket by id", async () => {
    await cacheTickets([ticket("t-1")]);
    expect((await loadCachedTicket("t-1"))?.ticket_id).toBe("t-1");
    expect(await loadCachedTicket("missing")).toBeUndefined();
  });
});
