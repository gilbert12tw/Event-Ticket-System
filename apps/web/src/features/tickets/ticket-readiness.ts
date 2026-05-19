import type { Ticket } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { siteLabel } from "@/lib/ui/options";

export type TicketReadinessView = {
  kind:
    | "entry-ready"
    | "not-open"
    | "qr-pending"
    | "redeemed"
    | "revoked"
    | "unavailable";
  label: string;
  tone: "ok" | "warn" | "fail" | "neutral" | "info";
  copy: string;
};

export function ticketEntryReadinessView(ticket: Ticket): TicketReadinessView {
  if (ticket.status === "active") {
    if (
      ticket.event_starts_at &&
      new Date(ticket.event_starts_at) > new Date()
    ) {
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
