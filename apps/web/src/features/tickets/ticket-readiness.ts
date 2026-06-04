import type { Ticket } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { siteLabel } from "@/lib/ui/options";

const fallbackTicketWindowMs = 24 * 60 * 60 * 1000;

export type TicketReadinessView = {
  kind:
    | "entry-ready"
    | "not-open"
    | "qr-pending"
    | "expired"
    | "redeemed"
    | "revoked"
    | "unavailable";
  label: string;
  tone: "ok" | "warn" | "fail" | "neutral" | "info";
  copy: string;
};

export function ticketEntryReadinessView(
  ticket: Ticket,
  now: Date = new Date(),
): TicketReadinessView {
  if (ticket.status === "active") {
    const startsAt = parseTime(ticket.event_starts_at);
    const expiresAt = ticketExpiryTime(ticket);
    if (expiresAt > 0 && expiresAt <= now.getTime()) {
      return {
        kind: "expired",
        label: "已過期",
        tone: "neutral",
        copy: "此票券已超過有效期限，不能入場。",
      };
    }
    if (startsAt > now.getTime()) {
      return {
        kind: "not-open",
        label: "尚未開放入場",
        tone: "info",
        copy: "活動尚未開始，請於開始時間到場後再出示二維碼驗票。",
      };
    }
    if (ticket.qr_payload || ticket.signed_token) {
      return {
        kind: "entry-ready",
        label: "可入場",
        tone: "ok",
        copy: "入場時出示此二維碼。票券安全碼已隱藏，請勿截圖轉傳。",
      };
    }
    return {
      kind: "qr-pending",
      label: "待產生 QR",
      tone: "warn",
      copy: "二維碼尚未產生，請重新整理票券；若仍未出現，請聯絡活動主辦。",
    };
  }
  if (ticket.status === "redeemed") {
    return {
      kind: "redeemed",
      label: "不可入場",
      tone: "neutral",
      copy: "此票券已核銷，不能再次入場。",
    };
  }
  if (ticket.status === "revoked") {
    return {
      kind: "revoked",
      label: "不可入場",
      tone: "fail",
      copy: "此票券已撤銷，不能入場。",
    };
  }
  return {
    kind: "unavailable",
    label: "不可入場",
    tone: "neutral",
    copy: "此票券目前不可用，請聯絡活動主辦。",
  };
}

export function ticketListMeta(ticket: Ticket) {
  const time = ticket.event_starts_at || ticket.issued_at;
  const location = ticket.event_location
    ? siteLabel(ticket.event_location)
    : "";
  return [time ? formatDate(time) : "", location].filter(Boolean).join(" · ");
}

export function safeTicketID(ticketID: string) {
  if (ticketID.length <= 24) return ticketID;
  return `${ticketID.slice(0, 12)}...${ticketID.slice(-6)}`;
}

export function selectCurrentTicket(
  tickets: Ticket[],
  now: Date = new Date(),
): Ticket | undefined {
  return tickets
    .filter(
      (ticket) => ticketEntryReadinessView(ticket, now).kind === "entry-ready",
    )
    .sort((left, right) => {
      const leftStart = parseTime(left.event_starts_at);
      const rightStart = parseTime(right.event_starts_at);
      return rightStart - leftStart;
    })[0];
}

function ticketExpiryTime(ticket: Ticket) {
  const explicitExpiry = parseTime(ticket.expires_at);
  const startsAt = parseTime(ticket.event_starts_at);
  const fallbackExpiry = startsAt > 0 ? startsAt + fallbackTicketWindowMs : 0;
  if (explicitExpiry > 0 && fallbackExpiry > 0) {
    return Math.min(explicitExpiry, fallbackExpiry);
  }
  if (explicitExpiry > 0) return explicitExpiry;
  return fallbackExpiry;
}

function parseTime(value?: string) {
  if (!value) return 0;
  const time = new Date(value).getTime();
  return Number.isNaN(time) ? 0 : time;
}
