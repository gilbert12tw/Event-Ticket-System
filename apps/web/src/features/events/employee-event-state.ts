import type { EventSummary } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import {
  eligibilityReasonLabel,
  eventStatusView,
  registrationStatusView,
  ticketStatusView,
  type Tone,
} from "@/lib/ui/options";

export type AttendeeActionKind = "book" | "waitlist" | "ticket" | "blocked";

export type AttendeeActionState = {
  kind: AttendeeActionKind;
  label: string;
  enabled: boolean;
  tone: Tone;
  recoveryCopy: string;
};

export function attendeeActionState(event: EventSummary): AttendeeActionState {
  const now = Date.now();
  const registrationStart = Date.parse(event.registration_start);
  const registrationClose = Date.parse(event.registration_close);
  const ticket = event.current_user_ticket;

  if (ticket?.status === "active") {
    return {
      kind: "ticket",
      label: "查看票券",
      enabled: true,
      tone: "ok",
      recoveryCopy: "票券已核發，入場時出示二維碼。",
    };
  }

  if (event.current_user_status === "confirmed") {
    return {
      kind: "blocked",
      label: "已報名",
      enabled: false,
      tone: "ok",
      recoveryCopy: "報名已確認，票券核發後會出現在我的票券。",
    };
  }

  if (event.current_user_status === "waitlisted") {
    return {
      kind: "blocked",
      label: "候補中",
      enabled: false,
      tone: "warn",
      recoveryCopy: "已有候補紀錄，有名額釋出時會通知你。",
    };
  }

  if (event.no_show_cooldown?.active) {
    return {
      kind: "blocked",
      label: "暫停報名",
      enabled: false,
      tone: "warn",
      recoveryCopy: `缺席冷卻期間暫停限量活動報名，開放時間：${formatDate(
        event.no_show_cooldown.until || "",
      )}。`,
    };
  }

  if (!event.eligible) {
    return {
      kind: "blocked",
      label: "不符合資格",
      enabled: false,
      tone: "fail",
      recoveryCopy: `你目前不符合資格：${eligibilityReasonLabel(
        event.eligibility_reason,
        event.eligible,
      )}。`,
    };
  }

  if (event.status !== "published") {
    return {
      kind: "blocked",
      label: eventStatusView(event.status).label,
      enabled: false,
      tone: eventStatusView(event.status).tone,
      recoveryCopy: eventStatusRecovery(event.status),
    };
  }

  if (Number.isFinite(registrationStart) && now < registrationStart) {
    return {
      kind: "blocked",
      label: "尚未開放",
      enabled: false,
      tone: "neutral",
      recoveryCopy: `報名尚未開始，${formatDate(event.registration_start)} 開放。`,
    };
  }

  if (Number.isFinite(registrationClose) && now > registrationClose) {
    return {
      kind: "blocked",
      label: "報名截止",
      enabled: false,
      tone: "warn",
      recoveryCopy: "報名期間已結束，請聯絡活動主辦。",
    };
  }

  if (event.capacity_type === "unlimited") {
    return {
      kind: "book",
      label: "立即報名",
      enabled: true,
      tone: "ok",
      recoveryCopy: "不限量活動會立即送出報名，家屬人數會記錄在報名資料。",
    };
  }

  if ((event.remaining_capacity ?? 0) > 0) {
    return {
      kind: "book",
      label: "立即報名",
      enabled: true,
      tone: "ok",
      recoveryCopy: `${event.remaining_capacity ?? 0} 席可報名，送出後會重新確認資格與名額。`,
    };
  }

  return {
    kind: "waitlist",
    label: "加入候補",
    enabled: true,
    tone: "warn",
    recoveryCopy: "活動已額滿，送出後會加入候補名單。",
  };
}

export function canSubmitAttendeeAction(event: EventSummary) {
  const action = attendeeActionState(event);
  return (
    action.enabled && (action.kind === "book" || action.kind === "waitlist")
  );
}

export function eligibilityDecisionLabel(event: EventSummary) {
  return eligibilityReasonLabel(event.eligibility_reason, event.eligible);
}

export function availabilityDecisionLabel(event: EventSummary) {
  if (event.capacity_type === "unlimited") return "不限量，可報名";
  const remaining = event.remaining_capacity ?? 0;
  if (remaining > 0) return `${remaining} 席可報名`;
  return "已額滿，可候補";
}

export function bookingWindowLabel(event: EventSummary) {
  return `${formatDate(event.registration_start)} 至 ${formatDate(
    event.registration_close,
  )}`;
}

export function attendeeStatusLabel(event: EventSummary) {
  if (event.current_user_ticket) {
    return ticketStatusView(event.current_user_ticket.status).label;
  }
  if (event.current_user_status) {
    return registrationStatusView(event.current_user_status).label;
  }
  return "未報名";
}

function eventStatusRecovery(status: string) {
  if (status === "draft") return "活動尚未發布，請稍後再查看。";
  if (status === "closed") return "活動已關閉，請聯絡活動主辦。";
  if (status === "cancelled") return "活動已取消，不能報名。";
  if (status === "archived") return "活動已封存，不能報名。";
  return "活動目前不可報名。";
}
