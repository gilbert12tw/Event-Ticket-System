import { navigate } from "@/app/routes";
import { Alert, Kpi, ProgressMeter, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import type { EventSummary } from "@/lib/api";
import { getEligibilityDecision } from "@/lib/api/contracts";
import {
  bookingActionLabel,
  capacityTypeLabel,
  eventStatusLabel,
  eventStatusTone,
  formatDate,
  registrationStatusLabel,
  registrationTone,
} from "@/lib/formatting";
import { EligibilityWarningList } from "./eligibility-warning";

const maxFamilyCount = 10;

export function EmployeeEventCard({
  cancelReason,
  event,
  familyCount,
  onBook,
  onCancel,
  onCancelReasonChange,
  onFamilyCountChange,
}: {
  cancelReason: string;
  event: EventSummary;
  familyCount: number;
  onBook: () => void;
  onCancel: () => void;
  onCancelReasonChange: (value: string) => void;
  onFamilyCountChange: (value: number) => void;
}) {
  return (
    <article className="event-card">
      <EventSummaryBlock event={event} compact />
      <div className="event-action">
        <Kpi
          label="容量"
          value={
            event.capacity_type === "limited" ? (event.capacity ?? 0) : "不限"
          }
        />
        <Kpi label="剩餘" value={event.remaining_capacity ?? "不限"} />
        <Kpi label="候補" value={event.waitlist_count} />
        <FamilyCountControl
          event={event}
          value={familyCount}
          onChange={onFamilyCountChange}
        />
        <button
          className="button"
          type="button"
          disabled={!canBook(event)}
          onClick={onBook}
        >
          <Icon name="ticket" />
          {bookingActionLabel(event)}
        </button>
        <button
          className="button secondary"
          type="button"
          onClick={() =>
            navigate(
              `/user/events/detail?event_id=${encodeURIComponent(event.event_id)}`,
            )
          }
        >
          <Icon name="audit" />
          詳情
        </button>
        <CancellationControl
          event={event}
          reason={cancelReason}
          onCancel={onCancel}
          onReasonChange={onCancelReasonChange}
        />
      </div>
    </article>
  );
}

export function EventSummaryBlock({
  compact = false,
  event,
}: {
  compact?: boolean;
  event: EventSummary;
}) {
  const capacityMax =
    event.capacity ?? Math.max(event.confirmed_count + event.waitlist_count, 1);
  const cooldown = event.no_show_cooldown;
  const eligibilityDecision = getEligibilityDecision(event);
  const warnings = eligibilityDecision?.warnings ?? [];
  const ineligibleReasons = eligibilityDecision?.reasons ?? [];
  return (
    <div className={compact ? "" : "summary-block"}>
      <div className="event-card-top">
        <StatusBadge tone={event.eligible && !cooldown?.active ? "ok" : "fail"}>
          {eligibilityLabel(event)}
        </StatusBadge>
        <StatusBadge
          tone={event.capacity_type === "unlimited" ? "info" : "neutral"}
        >
          {capacityTypeLabel(event.capacity_type)}
        </StatusBadge>
        <StatusBadge tone={eventStatusTone(event.status)}>
          {eventStatusLabel(event.status)}
        </StatusBadge>
        {event.current_user_status && (
          <StatusBadge tone={registrationTone(event.current_user_status)}>
            {registrationStatusLabel(event.current_user_status)}
          </StatusBadge>
        )}
      </div>
      <h3>{event.title}</h3>
      <p>{event.description || "此活動尚未填寫描述。"}</p>
      {cooldown?.active && (
        <Alert tone="warn">
          限量活動報名因未報到冷卻而暫停，直到{" "}
          {formatDisplayDate(cooldown.until || "")}。
        </Alert>
      )}
      {eligibilityDecision && !eligibilityDecision.eligible && (
        <Alert tone="fail">
          <strong>Not eligible:</strong>
          <p>
            {ineligibleReasons.length > 0
              ? ineligibleReasons.join(", ")
              : "Eligibility conditions are not met."}
          </p>
        </Alert>
      )}
      <EligibilityWarningList warnings={warnings} />
      <dl className="meta-list">
        {!compact && (
          <div>
            <dt>活動 ID</dt>
            <dd>{event.event_id}</dd>
          </div>
        )}
        <div>
          <dt>地點</dt>
          <dd>{event.location || event.event_site || "未設定"}</dd>
        </div>
        <div>
          <dt>開始時間</dt>
          <dd>{formatDisplayDate(event.starts_at)}</dd>
        </div>
        <div>
          <dt>報名截止</dt>
          <dd>{formatDisplayDate(event.registration_close)}</dd>
        </div>
        <div>
          <dt>資格規則</dt>
          <dd>
            {event.rule.department} / {event.rule.site} / G
            {event.rule.min_grade}+
          </dd>
        </div>
      </dl>
      {event.capacity_type === "limited" ? (
        <ProgressMeter
          label="容量使用"
          value={event.confirmed_count}
          max={capacityMax}
          helper={`${event.confirmed_count}/${capacityMax} 已確認，候補 ${event.waitlist_count}`}
        />
      ) : (
        <p className="form-hint">
          不限量活動：報名不會扣除庫存，也不會為同行人另發票券。
        </p>
      )}
    </div>
  );
}

export function FamilyCountControl({
  event,
  onChange,
  value,
}: {
  event: EventSummary;
  onChange: (value: number) => void;
  value: number;
}) {
  if (event.capacity_type === "limited") {
    return <p className="form-hint">限量活動不開放同行人數。</p>;
  }
  if (!event.allows_family) {
    return <p className="form-hint">此不限量活動不開放同行人數。</p>;
  }
  const boundedValue = Math.min(Math.max(value, 0), maxFamilyCount);
  return (
    <label className="field compact">
      <span>同行人數</span>
      <input
        aria-label={`同行人數：${event.title}`}
        max={maxFamilyCount}
        min={0}
        onChange={(input) =>
          onChange(
            Math.min(
              Math.max(Number(input.target.value || 0), 0),
              maxFamilyCount,
            ),
          )
        }
        type="number"
        value={boundedValue}
      />
      <small className="form-hint">
        可填 0 到 {maxFamilyCount} 人；同行人數會記錄在你的主要票券。
      </small>
    </label>
  );
}

export function CancellationControl({
  event,
  onCancel,
  onReasonChange,
  reason,
}: {
  event: EventSummary;
  onCancel: () => void;
  onReasonChange: (value: string) => void;
  reason: string;
}) {
  const registrationID = registrationIDFor(event);
  const booked =
    event.current_user_status === "confirmed" ||
    event.current_user_status === "waitlisted";
  if (!booked) return <p className="form-hint">目前沒有可取消的報名。</p>;
  const open = cancellationOpen(event);
  return (
    <div className="cancel-box">
      <label className="field compact">
        <span>取消原因</span>
        <input
          value={reason}
          onChange={(input) => onReasonChange(input.target.value)}
          placeholder="可選填原因"
          disabled={!open}
        />
      </label>
      <button
        aria-label="取消報名"
        className="button secondary"
        type="button"
        onClick={onCancel}
        disabled={!open || !registrationID}
      >
        <Icon name="x" />
        取消報名
      </button>
      <p className="form-hint">
        {open
          ? "報名開放期間可安全重試取消報名。"
          : "自助取消已關閉，請聯絡活動管理員處理例外。"}
      </p>
    </div>
  );
}

export function canBook(event: EventSummary) {
  const eligibilityDecision = getEligibilityDecision(event);
  const isEligible = eligibilityDecision
    ? eligibilityDecision.can_book
    : event.eligible;
  const cooldown =
    eligibilityDecision?.no_show_cooldown ?? event.no_show_cooldown;
  return (
    isEligible &&
    !cooldown?.active &&
    event.current_user_status !== "confirmed" &&
    event.current_user_status !== "waitlisted"
  );
}

export function registrationIDFor(event: EventSummary) {
  return (
    event.current_user_registration_id ||
    event.current_user_ticket?.registration_id ||
    ""
  );
}

export function messageTone(message: string): "ok" | "warn" | "fail" | "info" {
  const lower = message.toLowerCase();
  if (lower.includes("cancelled") || lower.includes("confirmed")) return "ok";
  if (
    lower.includes("cooldown") ||
    lower.includes("closed") ||
    lower.includes("waitlist")
  )
    return "warn";
  if (
    lower.includes("error") ||
    lower.includes("failed") ||
    lower.includes("not eligible")
  )
    return "fail";
  return "info";
}

function eligibilityLabel(event: EventSummary) {
  const reason = (event.eligibility_reason ?? "").trim();
  if (!reason) return event.eligible ? "符合資格" : "不可報名";
  if (reason === "eligible") return "符合資格";
  return reason;
}

function cancellationOpen(event: EventSummary) {
  const close = Date.parse(event.registration_close);
  return Number.isFinite(close) && close > Date.now();
}

function formatDisplayDate(value?: string | null) {
  return value ? formatDate(value) : "未設定";
}
