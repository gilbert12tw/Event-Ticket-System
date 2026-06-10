import type { Ticket } from "@/lib/api";
import { clearStore, getAllItems, getItem, putItem } from "./db";

const STORE = "tickets";

export async function cacheTickets(tickets: Ticket[]): Promise<void> {
  await clearStore(STORE);
  for (const ticket of tickets) {
    await putItem(STORE, ticket);
  }
}

export async function loadCachedTickets(): Promise<Ticket[]> {
  return getAllItems<Ticket>(STORE);
}

export async function loadCachedTicket(
  ticketID: string,
): Promise<Ticket | undefined> {
  return getItem<Ticket>(STORE, ticketID);
}
