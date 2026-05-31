import { navigate, ticketDetailPath } from "@/app/routes";
import { Alert, MetaList } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import type { AuthMeClaims, BookingResponse, EventSummary } from "@/lib/api";
import { formatDate } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import { localizedMessage } from "@/lib/ui/options";
import { Button } from "@/components/ui/button";
import {
  attendeeActionState,
  attendeeStatusLabel,
  availabilityDecisionLabel,
  eligibilityDecisionLabel,
} from "./employee-event-state";

export type BookingResultState = {
  title: string;
  copy: string;
  tone: "ok" | "warn" | "fail" | "info";
  ticketID?: string;
};

export function BookingResultBlock({
  eventID,
  result,
}: Readonly<{
  eventID: string;
  result?: BookingResultState;
}>) {
  if (!result) return null;
  return (
    <div
      className={`booking-result ${result.tone}`}
      id={`booking-result-${eventID}`}
      aria-live="polite"
      role="status"
      tabIndex={-1}
    >
      <Alert tone={result.tone}>{result.title}</Alert>
      <p>{result.copy}</p>
      <BookingResultAction result={result} />
    </div>
  );
}

export function ActionSummary({
  claims,
  event,
  familyCount,
}: Readonly<{
  claims: AuthMeClaims;
  event: EventSummary;
  familyCount: number;
}>) {
  const action = attendeeActionState(event);
  return (
    <div className="pre-submit-summary">
      <div className="section-heading compact">
        <div>
          <h3>送出前確認</h3>
          <p>{action.recoveryCopy}</p>
        </div>
      </div>
      <MetaList
        rows={[
          ["活動", event.title],
          ["報名人", claims.display_name || claims.employee_id],
          ["資格", eligibilityDecisionLabel(event)],
          ["名額結果", availabilityDecisionLabel(event)],
          ["取消期限", formatDate(event.registration_close)],
          ...(event.capacity_type === "unlimited"
            ? ([["同行家屬", `${familyCount} 人`]] as const)
            : []),
          ["目前狀態", attendeeStatusLabel(event)],
        ]}
      />
      <p className="form-hint">
        送出時系統會重新檢查資格、活動狀態與名額；額滿時會依活動政策加入候補。
      </p>
    </div>
  );
}

export function bookingResultFromResponse(
  response: BookingResponse,
): BookingResultState {
  const message = localizedMessage(response.message);
  if (response.duplicate) {
    return duplicateBookingResult(response);
  }
  if (response.ticket) {
    return {
      title: "報名成功，票券已核發",
      copy: "票券已可使用。入場時請開啟我的票券並出示二維碼。",
      tone: "ok",
      ticketID: response.ticket.ticket_id,
    };
  }
  if (response.registration.status === "waitlisted") {
    return {
      title: "已加入候補",
      copy: "有名額釋出時會通知你。目前不顯示候補順位，系統會依活動政策與通知中心更新狀態。",
      tone: "warn",
    };
  }
  return {
    title: message.includes("候補") ? "已加入候補" : "報名結果已更新",
    copy: message,
    tone: message.includes("候補") ? "warn" : "info",
  };
}

export function bookingResultFromExistingEvent(
  event: EventSummary,
): BookingResultState | null {
  if (event.current_user_status === "cancelled") {
    return {
      title: "報名已取消",
      copy: "系統找到已取消的既有報名，未建立新的報名；若需恢復或重新報名，請聯絡活動主辦。",
      tone: "warn",
    };
  }
  if (event.current_user_ticket?.status === "active") {
    return {
      title: "你已經報名此活動",
      copy: "系統找到既有報名，未建立新的報名。入場時請開啟我的票券並出示二維碼。",
      tone: "info",
      ticketID: event.current_user_ticket.ticket_id,
    };
  }
  if (event.current_user_status === "confirmed") {
    return {
      title: "你已經報名此活動",
      copy: "系統找到既有報名，未建立新的報名。票券核發後會出現在我的票券。",
      tone: "info",
    };
  }
  if (event.current_user_status === "waitlisted") {
    return {
      title: "你已在候補中",
      copy: "系統找到既有候補紀錄，未建立新的候補。名額釋出時會透過通知中心更新。",
      tone: "warn",
    };
  }
  return null;
}

export function focusBookingResult(eventID: string) {
  globalThis.setTimeout(() => {
    document.getElementById(`booking-result-${eventID}`)?.focus();
  }, 0);
}

function duplicateBookingResult(response: BookingResponse): BookingResultState {
  if (response.registration.status === "cancelled") {
    return {
      title: "報名已取消",
      copy: "系統找到已取消的既有報名，未建立新的報名；若需恢復或重新報名，請聯絡活動主辦。",
      tone: "warn",
    };
  }
  if (response.ticket?.status === "active") {
    return {
      title: "你已經報名此活動",
      copy: "系統找到既有報名，未建立新的報名。入場時請開啟我的票券並出示二維碼。",
      tone: "info",
      ticketID: response.ticket.ticket_id,
    };
  }
  if (response.registration.status === "waitlisted") {
    return {
      title: "你已在候補中",
      copy: "系統找到既有候補紀錄，未建立新的候補。名額釋出時會透過通知中心更新。",
      tone: "warn",
    };
  }
  return {
    title: "你已經報名此活動",
    copy: "系統找到既有報名，未建立新的報名。請重新整理活動詳情確認目前狀態。",
    tone: "info",
  };
}

function BookingResultAction({
  result,
}: Readonly<{ result: BookingResultState }>) {
  const ticketID = result.ticketID;
  if (ticketID) {
    return (
      <Button asChild variant="outline">
        <a
          href={ticketDetailPath(ticketID)}
          onClick={(event) =>
            runClientNavigation(event, () =>
              navigate(ticketDetailPath(ticketID)),
            )
          }
        >
          <Icon name="ticket" />
          查看票券
        </a>
      </Button>
    );
  }
  if (result.title.includes("候補")) {
    return (
      <Button asChild variant="outline">
        <a
          href="/user/notifications"
          onClick={(event) =>
            runClientNavigation(event, () => navigate("/user/notifications"))
          }
        >
          <Icon name="send" />
          查看通知中心
        </a>
      </Button>
    );
  }
  return (
    <Button asChild variant="outline">
      <a
        href="/user/events?tab=registered"
        onClick={(event) =>
          runClientNavigation(event, () =>
            navigate("/user/events?tab=registered"),
          )
        }
      >
        <Icon name="calendar" />
        查看我的報名
      </a>
    </Button>
  );
}
