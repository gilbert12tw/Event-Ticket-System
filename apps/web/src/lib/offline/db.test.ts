import { beforeEach, describe, expect, it } from "vitest";
import { IDBFactory } from "fake-indexeddb";
import {
  clearStore,
  deleteItem,
  getAllItems,
  getItem,
  openDB,
  putItem,
  resetCachedDB,
} from "./db";

type TicketRow = { ticket_id: string; status: string };

beforeEach(() => {
  // Fresh IndexedDB universe per test so stores never leak across cases.
  globalThis.indexedDB = new IDBFactory();
  resetCachedDB();
});

describe("openDB", () => {
  it("creates the tickets and checkin-packages stores on first open", async () => {
    const db = await openDB();

    expect(Array.from(db.objectStoreNames)).toEqual(
      expect.arrayContaining(["tickets", "checkin-packages"]),
    );
  });

  it("returns the same cached connection on repeated opens", async () => {
    const first = await openDB();
    const second = await openDB();

    expect(second).toBe(first);
  });

  it("opens a new connection after resetCachedDB", async () => {
    const first = await openDB();
    resetCachedDB();
    const second = await openDB();

    expect(second).not.toBe(first);
  });
});

describe("item operations", () => {
  it("putItem then getItem round-trips a record", async () => {
    const ticket: TicketRow = { ticket_id: "t-1", status: "issued" };
    await putItem("tickets", ticket);

    await expect(getItem<TicketRow>("tickets", "t-1")).resolves.toEqual(
      ticket,
    );
  });

  it("putItem overwrites an existing record with the same key", async () => {
    await putItem("tickets", { ticket_id: "t-1", status: "issued" });
    await putItem("tickets", { ticket_id: "t-1", status: "redeemed" });

    const stored = await getItem<TicketRow>("tickets", "t-1");
    expect(stored?.status).toBe("redeemed");
  });

  it("getItem resolves undefined for a missing key", async () => {
    await expect(getItem("tickets", "missing")).resolves.toBeUndefined();
  });

  it("getAllItems returns every stored record", async () => {
    await putItem("tickets", { ticket_id: "t-1", status: "issued" });
    await putItem("tickets", { ticket_id: "t-2", status: "issued" });

    const all = await getAllItems<TicketRow>("tickets");
    expect(all.map((t) => t.ticket_id).sort()).toEqual(["t-1", "t-2"]);
  });

  it("deleteItem removes only the targeted record", async () => {
    await putItem("tickets", { ticket_id: "t-1", status: "issued" });
    await putItem("tickets", { ticket_id: "t-2", status: "issued" });

    await deleteItem("tickets", "t-1");

    await expect(getItem("tickets", "t-1")).resolves.toBeUndefined();
    await expect(getItem("tickets", "t-2")).resolves.toBeDefined();
  });

  it("clearStore empties the store", async () => {
    await putItem("checkin-packages", { batch_id: "b-1" });
    await putItem("checkin-packages", { batch_id: "b-2" });

    await clearStore("checkin-packages");

    await expect(getAllItems("checkin-packages")).resolves.toEqual([]);
  });

  it("rejects when the object store does not exist", async () => {
    await expect(putItem("nope", { ticket_id: "t-1" })).rejects.toThrow();
    await expect(getItem("nope", "t-1")).rejects.toThrow();
    await expect(getAllItems("nope")).rejects.toThrow();
    await expect(deleteItem("nope", "t-1")).rejects.toThrow();
    await expect(clearStore("nope")).rejects.toThrow();
  });

  it("rejects putItem when the record is missing the key path", async () => {
    await expect(putItem("tickets", { status: "issued" })).rejects.toThrow();
  });
});
