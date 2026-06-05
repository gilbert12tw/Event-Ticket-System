import type { ReactNode } from "react";
import { navigate, ticketDetailPath } from "@/app/routes";
import {
  Alert,
  EventListItem,
  Field,
  MetaList,
  ProgressMeter,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import type { EventSummary } from "@/lib/api";
import { getEligibilityDecision } from "@/lib/api/contracts";
import { formatDate } from "@/lib/formatting";
import { runClientNavigation } from "@/lib/navigation";
import {
  departmentLabel,
  employmentStatusLabel,
  eventStatusView,
  registrationStatusView,
  siteLabel,
  type Tone,
} from "@/lib/ui/options";
import { Button } from "@/components/ui/button";
import { CancellationControl } from "./employee-cancellation-control";
import {
  attendeeActionState,
  availabilityDecisionLabel,
  bookingWindowLabel,
  canSubmitAttendeeAction,
  eligibilityDecisionLabel,
} from "./employee-event-state";
import { EligibilityWarningList } from "./eligibility-warning";

const maxFamilyCount = 10;

export function EmployeeEventCard({
  cancelReason,
  cancelBusy = false,
  event,
  mode,
  onCancel,
  onCancelReasonChange,
  pending = false,
  result,
}: Readonly<{
  cancelReason?: string;
  cancelBusy?: boolean;
  event: EventSummary;
  mode: "available" | "registered" | "unavailable";
  onCancel?: () => void;
  onCancelReasonChange?: (value: string) => void;
  pending?: boolean;
  result?: ReactNode;
}>) {
  const action = attendeeActionState(event);
  const eligibilityDecision = getEligibilityDecision(event);
  const cooldown =
    eligibilityDecision?.no_show_cooldown ?? event.no_show_cooldown;
  const isEligible = eligibilityDecision
    ? eligibilityDecision.eligible
    : (event.eligible ?? false);
  const needsFamilySetup =
    action.kind !== "ticket" &&
    event.capacity_type === "unlimited" &&
    event.allows_family;
  const canRunPrimary =
    action.kind === "ticket" || canSubmitAttendeeAction(event);
  const eligibilityTone = isEligible && !cooldown?.active ? "ok" : "fail";
  const detailHref = `/user/events/detail?event_id=${encodeURIComponent(
    event.event_id,
  )}`;
  const primaryHref = resolvePrimaryHref(
    action.kind,
    detailHref,
    event.current_user_ticket,
  );
  return (
    <EventListItem
      title={event.title}
      meta={`${formatDate(event.starts_at)} 至 ${formatDate(event.ends_at)} · ${siteLabel(
        event.location || event.event_site,
      )}`}
      description={
        <EventCardDescription event={event} copy={action.recoveryCopy} />
      }
      badges={
        <EventBadges
          event={event}
          capacity={{ label: capacitySummaryLabel(event), tone: action.tone }}
          eligibilityTone={eligibilityTone}
          showRegistration={mode !== "available"}
        />
      }
      actions={
        <>
          {canRunPrimary && (
            <Button asChild>
              <a
                aria-disabled={pending || undefined}
                href={primaryHref}
                onClick={(clickEvent) => {
                  if (pending) {
                    clickEvent.preventDefault();
                    return;
                  }
                  runClientNavigation(clickEvent, () => navigate(primaryHref));
                }}
              >
                <Icon name={needsFamilySetup ? "calendar" : "ticket"} />
                {needsFamilySetup ? "設定同行人數" : action.label}
              </a>
            </Button>
          )}
          {!canRunPrimary && <BlockedEventAction event={event} />}
          <Button asChild size="sm" variant="ghost">
            <a
              aria-label={`查看活動詳情：${event.title}`}
              href={detailHref}
              onClick={(clickEvent) =>
                runClientNavigation(clickEvent, () => navigate(detailHref))
              }
            >
              詳情
            </a>
          </Button>
          {result}
          {mode === "registered" && onCancel && onCancelReasonChange && (
            <CancellationControl
              busy={cancelBusy}
              event={event}
              reason={cancelReason || ""}
              onCancel={onCancel}
              onReasonChange={onCancelReasonChange}
            />
          )}
          {event.no_show_cooldown?.active && (
            <Alert tone="warn">
              限量活動因缺席冷卻期暫停報名，開放時間：
              {formatDate(event.no_show_cooldown.until || "")}。
            </Alert>
          )}
        </>
      }
    />
  );
}

function EventCardDescription({
  copy,
  event,
}: Readonly<{
  copy: string;
  event: EventSummary;
}>) {
  const warnings = getEligibilityDecision(event)?.warnings ?? [];
  return (
    <div className="event-card-description-stack">
      <span>{copy}</span>
      <EligibilityWarningList warnings={warnings} />
    </div>
  );
}

function BlockedEventAction({ event }: Readonly<{ event: EventSummary }>) {
  const action = attendeeActionState(event);
  return (
    <Button
      aria-label={`${action.label}：${action.recoveryCopy}`}
      className="blocked-action-button"
      disabled
      title={action.recoveryCopy}
      type="button"
      variant="outline"
    >
      <Icon name="ban" />
      {action.label}
    </Button>
  );
}

function EventBadges({
  capacity,
  eligibilityTone,
  event,
  showRegistration = true,
}: Readonly<{
  capacity: { label: string; tone: Tone };
  eligibilityTone: Tone;
  event: EventSummary;
  showRegistration?: boolean;
}>) {
  const eventStatus = eventStatusView(event.status);
  const registrationStatus = event.current_user_status
    ? registrationStatusView(event.current_user_status)
    : null;
  return (
    <>
      <StatusBadge tone={eligibilityTone}>
        {eligibilityLabel(event)}
      </StatusBadge>
      <StatusBadge tone={capacity.tone}>{capacity.label}</StatusBadge>
      <StatusBadge tone={eventStatus.tone}>{eventStatus.label}</StatusBadge>
      {showRegistration && registrationStatus && (
        <StatusBadge tone={registrationStatus.tone}>
          {registrationStatus.label}
        </StatusBadge>
      )}
    </>
  );
}

export function EventSummaryBlock({
  compact = false,
  event,
}: Readonly<{
  compact?: boolean;
  event: EventSummary;
}>) {
  const capacityMax =
    event.capacity ?? Math.max(event.confirmed_count + event.waitlist_count, 1);
  const eligibilityDecision = getEligibilityDecision(event);
  const cooldown =
    eligibilityDecision?.no_show_cooldown ?? event.no_show_cooldown;
  const action = attendeeActionState(event);
  const warnings = eligibilityDecision?.warnings ?? [];
  const ineligibleReasons = eligibilityDecision?.reasons ?? [];
  const isEligible = eligibilityDecision
    ? eligibilityDecision.eligible
    : event.eligible;
  const eligibilityTone = isEligible && !cooldown?.active ? "ok" : "fail";
  return (
    <div className={compact ? "" : "summary-block"}>
      <div className="event-card-top">
        <EventBadges
          event={event}
          capacity={{
            label: event.capacity_type === "unlimited" ? "不限量" : "限量",
            tone: event.capacity_type === "unlimited" ? "info" : "neutral",
          }}
          eligibilityTone={eligibilityTone}
        />
      </div>
      <h3>{event.title}</h3>
      <p>{userFacingEventDescription(event.description)}</p>
      <div className="decision-strip" aria-label="活動可報名狀態">
        <div>
          <StatusBadge tone={isEligible && !cooldown?.active ? "ok" : "fail"}>
            資格
          </StatusBadge>
          <span>{eligibilityDecisionLabel(event)}</span>
        </div>
        <div>
          <StatusBadge tone={action.tone}>名額</StatusBadge>
          <span>{availabilityDecisionLabel(event)}</span>
        </div>
        <div>
          <StatusBadge tone="neutral">報名期間</StatusBadge>
          <span>{bookingWindowLabel(event)}</span>
        </div>
      </div>
      {cooldown?.active && (
        <Alert tone="warn">
          限量活動因缺席冷卻期暫停報名，開放時間：
          {formatDate(cooldown.until || "")}。
        </Alert>
      )}
      {eligibilityDecision && !eligibilityDecision.eligible && (
        <Alert tone="fail">
          <strong>不符合資格：</strong>
          <p>
            {ineligibleReasons.length > 0
              ? ineligibleReasons.join(", ")
              : "目前不符合活動資格條件。"}
          </p>
        </Alert>
      )}
      <EligibilityWarningList warnings={warnings} />
      <MetaList className="" rows={eventSummaryRows(event, compact)} />
      {event.capacity_type === "limited" ? (
        <ProgressMeter
          label="容量使用"
          value={event.confirmed_count}
          max={capacityMax}
          helper={`${event.confirmed_count}/${capacityMax} 已報名，${event.waitlist_count} 候補`}
        />
      ) : (
        <p className="form-hint">
          不限量活動不扣庫存，家屬人數請在詳情頁確認後再送出。
        </p>
      )}
    </div>
  );
}

function eventSummaryRows(
  event: EventSummary,
  compact: boolean,
): Array<[string, ReactNode]> {
  const rows: Array<[string, ReactNode]> = [];
  if (!compact) rows.push(["活動編號", event.event_id]);
  rows.push(
    ["地點", siteLabel(event.location || event.event_site)],
    ["開始時間", formatDate(event.starts_at)],
    ["結束時間", formatDate(event.ends_at)],
    ["報名開始", formatDate(event.registration_start)],
    ["報名截止", formatDate(event.registration_close)],
    [
      "資格規則",
      `${departmentLabel(event.rule.department)} / ${siteLabel(event.rule.site)} / G${event.rule.min_grade}+ / ${employmentStatusLabel(event.rule.employment_status)}`,
    ],
  );
  return rows;
}

export function WaitlistPolicy({ event }: Readonly<{ event: EventSummary }>) {
  if (event.capacity_type !== "limited") return null;
  const full = (event.remaining_capacity ?? 0) <= 0;
  if (!full && event.waitlist_count === 0) return null;
  return (
    <div className="helper-strip">
      <StatusBadge tone={full ? "warn" : "info"}>候補政策</StatusBadge>
      <span>{waitlistPolicyCopy(event)}</span>
    </div>
  );
}

export function FamilyCountControl({
  event,
  onChange,
  value,
}: Readonly<{
  event: EventSummary;
  onChange: (value: number) => void;
  value: number;
}>) {
  if (event.capacity_type === "limited") {
    return <p className="form-hint">限量活動不開放填寫家屬人數。</p>;
  }
  if (!event.allows_family) {
    return <p className="form-hint">此活動未開放攜帶家屬。</p>;
  }
  const boundedValue = Math.min(Math.max(value, 0), maxFamilyCount);
  return (
    <div className="compact-field">
      <Field
        inputMode="numeric"
        label="同行家屬"
        max={maxFamilyCount}
        min={0}
        name={`${event.event_id}-family-count`}
        type="number"
        value={String(boundedValue)}
        onChange={(input) =>
          onChange(Math.min(Math.max(Number(input || 0), 0), maxFamilyCount))
        }
        hint={`可填 0 到 ${maxFamilyCount} 人；人數會記錄在主要票券上。`}
      />
    </div>
  );
}

export function canBook(event: EventSummary) {
  return canSubmitAttendeeAction(event);
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
  if (
    lower.includes("cancelled") ||
    lower.includes("confirmed") ||
    message.includes("成功") ||
    message.includes("已取消") ||
    message.includes("已報名")
  )
    return "ok";
  if (
    lower.includes("cooldown") ||
    lower.includes("closed") ||
    lower.includes("waitlist") ||
    message.includes("冷卻") ||
    message.includes("已額滿") ||
    message.includes("候補")
  )
    return "warn";
  if (
    lower.includes("error") ||
    lower.includes("failed") ||
    lower.includes("not eligible") ||
    message.includes("失敗") ||
    message.includes("不符合")
  )
    return "fail";
  return "info";
}

function eligibilityLabel(event: EventSummary) {
  return eligibilityDecisionLabel(event);
}

function capacitySummaryLabel(event: EventSummary) {
  if (event.capacity_type === "unlimited") return "不限量";
  const remaining = event.remaining_capacity ?? 0;
  if (remaining > 0) return `${remaining} 席可報名`;
  if (event.waitlist_count > 0) return "候補中";
  return "已額滿";
}

function userFacingEventDescription(description?: string | null) {
  const cleaned = (description || "")
    .replaceAll(/phase\s*1/gi, "")
    .replaceAll("第一階段", "")
    .replaceAll("示範", "流程")
    .split(/\s+/)
    .filter(Boolean)
    .join(" ")
    .trim();
  return cleaned || "未提供活動描述。";
}

function resolvePrimaryHref(
  actionKind: string,
  detailHref: string,
  currentUserTicket?: { ticket_id: string } | null,
) {
  if (actionKind !== "ticket") return detailHref;
  if (currentUserTicket) return ticketDetailPath(currentUserTicket.ticket_id);
  return "/user/tickets";
}

function waitlistPolicyCopy(event: EventSummary) {
  const policy =
    event.allocation_mode === "lottery" ? "抽籤或管理員釋出" : "先到先處理";
  return `${policy}；目前不顯示候補順位，有名額釋出時會透過通知中心更新，保留期限依活動主辦政策處理。`;
}
