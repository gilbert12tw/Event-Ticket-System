import {
  Alert,
  EmptyState,
  ReadinessMessage,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import type { EmployeeProfile, EventSummary } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  categoryOptions,
  entryMethodOptions,
  eventStatusView,
  eventTemplateOptions,
  schedulePresetOptions,
  visibilityOptions,
} from "@/lib/ui/options";
import {
  EligibilityFields,
  EventCoreFields,
  IntentSelector,
  ReadinessGrid,
  SelectEventFirst,
  TagInput,
} from "./admin-event-form-controls";
import {
  applyEventTemplate,
  applySchedulePreset,
  type AdminCreateForm,
  type AdminEditForm,
  type AdminEventTab,
} from "./admin-event-crud-types";
export { AdminEventEligibilityTab } from "./admin-event-eligibility-tab";
export { AdminEventStatusTab } from "./admin-event-status-tab";

type FormSubmitHandler = (event: {
  preventDefault: () => void;
}) => void | Promise<void>;

export function AdminEventListTab({
  events,
  busy,
  selectedEvent,
  onRefresh,
  onSelect,
  onTabChange,
}: Readonly<{
  events: EventSummary[];
  busy: boolean;
  selectedEvent?: EventSummary;
  onRefresh: () => void;
  onSelect: (event: EventSummary) => void;
  onTabChange: (tab: AdminEventTab) => void;
}>) {
  return (
    <div className="task-panel">
      <div className="section-heading">
        <div>
          <h2>活動清單</h2>
          <p>只用來檢視、搜尋與選取活動；建立、編輯與封存都移到分頁。</p>
        </div>
        <Button
          variant="outline"
          type="button"
          onClick={onRefresh}
          disabled={busy}
        >
          <Icon name="refresh" />
          重新整理
        </Button>
      </div>
      {events.length === 0 ? (
        <EmptyState
          title="尚無管理活動"
          action="切換到建立活動分頁新增第一個活動。"
        />
      ) : (
        <div className="event-list compact-list">
          {events.map((event) => (
            <article
              className={
                selectedEvent?.event_id === event.event_id
                  ? "event-row active event-row-task"
                  : "event-row event-row-task"
              }
              key={event.event_id}
            >
              <Button
                className="event-row-button"
                type="button"
                aria-pressed={selectedEvent?.event_id === event.event_id}
                onClick={() => onSelect(event)}
                variant="ghost"
              >
                <span>
                  <strong>{event.title}</strong>
                  <small>{adminEventRowMeta(event)}</small>
                </span>
                <StatusBadge tone={eventStatusView(event.status).tone}>
                  {eventStatusView(event.status).label}
                </StatusBadge>
              </Button>
              <div className="row-actions">
                <Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onClick={() => {
                    onSelect(event);
                    onTabChange("edit");
                  }}
                >
                  編輯
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onClick={() => {
                    onSelect(event);
                    onTabChange("status");
                  }}
                >
                  狀態
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onClick={() => {
                    onSelect(event);
                    onTabChange("eligibility");
                  }}
                >
                  資格
                </Button>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}

export function AdminEventCreateTab({
  busy,
  canSubmit,
  capacityReady,
  form,
  message,
  previewEmployees,
  publishChecks,
  zeroAudiencePublishBlocked,
  windowReady,
  onFormChange,
  onReset,
  onSeed,
  onSubmit,
}: Readonly<{
  busy: boolean;
  canSubmit: boolean;
  capacityReady: boolean;
  form: AdminCreateForm;
  message: string;
  previewEmployees: EmployeeProfile[];
  publishChecks: { label: string; ok: boolean }[];
  zeroAudiencePublishBlocked: boolean;
  windowReady: boolean;
  onFormChange: (next: AdminCreateForm) => void;
  onReset: () => void;
  onSeed: () => void;
  onSubmit: FormSubmitHandler;
}>) {
  return (
    <form className="form-grid" onSubmit={onSubmit}>
      <div className="section-heading full">
        <div>
          <h2>建立活動</h2>
          <p>只處理新增活動，既有活動的編輯、狀態與封存移到其他分頁。</p>
        </div>
        <Button variant="ghost" type="button" onClick={onSeed} disabled={busy}>
          <Icon name="database" />
          載入起始人資
        </Button>
      </div>
      <fieldset className="form-section full">
        <legend>快速設定</legend>
        <SelectField
          label="活動模板"
          value=""
          options={[{ value: "", label: "選擇模板" }, ...eventTemplateOptions]}
          onChange={(value) => onFormChange(applyEventTemplate(form, value))}
        />
        <SelectField
          label="排程預設"
          value="custom"
          options={schedulePresetOptions}
          onChange={(value) => onFormChange(applySchedulePreset(form, value))}
        />
        <IntentSelector
          value={form.status}
          onChange={(status) => onFormChange({ ...form, status })}
        />
      </fieldset>
      <EventCoreFields
        form={form}
        onChange={onFormChange}
        windowReady={windowReady}
        capacityReady={capacityReady}
      />
      <EligibilityFields form={form} onChange={onFormChange} />
      <fieldset className="form-section full">
        <legend>投遞與標籤</legend>
        <SelectField
          label="分類"
          value={form.category}
          options={categoryOptions}
          onChange={(category) => onFormChange({ ...form, category })}
        />
        <SelectField
          label="入場方式"
          value={form.entry_method}
          options={entryMethodOptions}
          onChange={(entry_method) => onFormChange({ ...form, entry_method })}
        />
        <SelectField
          label="可見性"
          value={form.visibility}
          options={visibilityOptions}
          onChange={(visibility) => onFormChange({ ...form, visibility })}
        />
        <TagInput
          value={form.tags}
          onChange={(tags) => onFormChange({ ...form, tags })}
        />
      </fieldset>
      <ReadinessMessage
        tone={previewEmployees.length > 0 ? "ok" : "warn"}
        label={previewEmployees.length > 0 ? "資格命中" : "0 人符合"}
        message={readinessMessage(previewEmployees, form.status)}
      />
      {zeroAudiencePublishBlocked && (
        <Alert tone="warn">
          發布活動需要至少一位符合資格的員工。請改成草稿或調整資格規則。
        </Alert>
      )}
      <ReadinessGrid checks={publishChecks} />
      {!windowReady && (
        <Alert tone="warn">
          報名期間需早於活動開始，且報名開始不可晚於報名截止。
        </Alert>
      )}
      <div className="form-actions full">
        <Button type="submit" disabled={busy || !canSubmit}>
          <Icon name="plus" />
          {submitLabel(form.status, zeroAudiencePublishBlocked)}
        </Button>
        <Button variant="outline" type="button" onClick={onReset}>
          重設
        </Button>
      </div>
      {message && (
        <Alert tone={message.includes("已") ? "ok" : "warn"}>{message}</Alert>
      )}
    </form>
  );
}

export function AdminEventEditTab({
  busy,
  editForm,
  selectedEvent,
  windowReady,
  onEditFormChange,
  onSave,
}: Readonly<{
  busy: boolean;
  editForm: AdminEditForm;
  selectedEvent?: EventSummary;
  windowReady: boolean;
  onEditFormChange: (next: AdminEditForm) => void;
  onSave: FormSubmitHandler;
}>) {
  if (!selectedEvent) return <SelectEventFirst action="編輯" />;
  return (
    <form className="form-grid" onSubmit={onSave}>
      <div className="section-heading full">
        <div>
          <h2>編輯活動</h2>
          <p>只修改基本資料、時間、容量與投遞設定，不處理狀態或封存。</p>
        </div>
        <StatusBadge tone={eventStatusView(selectedEvent.status).tone}>
          {eventStatusView(selectedEvent.status).label}
        </StatusBadge>
      </div>
      <EventCoreFields
        form={editForm}
        onChange={onEditFormChange}
        windowReady={windowReady}
        capacityReady
      />
      <fieldset className="form-section full">
        <legend>投遞與標籤</legend>
        <SelectField
          label="分類"
          value={editForm.category}
          options={categoryOptions}
          onChange={(category) => onEditFormChange({ ...editForm, category })}
        />
        <SelectField
          label="入場方式"
          value={editForm.entry_method}
          options={entryMethodOptions}
          onChange={(entry_method) =>
            onEditFormChange({ ...editForm, entry_method })
          }
        />
        <SelectField
          label="可見性"
          value={editForm.visibility}
          options={visibilityOptions}
          onChange={(visibility) =>
            onEditFormChange({ ...editForm, visibility })
          }
        />
        <TagInput
          value={editForm.tags}
          onChange={(tags) => onEditFormChange({ ...editForm, tags })}
        />
      </fieldset>
      <div className="form-actions full">
        <Button type="submit" disabled={busy || !windowReady}>
          <Icon name="save" />
          儲存活動
        </Button>
      </div>
    </form>
  );
}

function adminEventRowMeta(event: EventSummary) {
  const capacity =
    event.capacity_type === "unlimited" ? "不限" : String(event.capacity ?? 0);
  const remaining =
    event.capacity_type === "unlimited"
      ? "不限"
      : String(event.remaining_capacity ?? 0);
  return `已報名 ${event.confirmed_count} / ${capacity} · 候補 ${event.waitlist_count} · 剩餘 ${remaining} · 規則版本 v${event.version || 1}`;
}

function submitLabel(status: string, zeroAudiencePublishBlocked: boolean) {
  if (zeroAudiencePublishBlocked) return "資格 0 人，不能發布";
  if (status === "draft") return "儲存草稿";
  return "建立並發布";
}

function readinessMessage(
  previewEmployees: EmployeeProfile[],
  eventStatus: string,
) {
  if (previewEmployees.length > 0) {
    return `符合：${previewEmployees
      .map((employee) => employee.employee_id)
      .join(", ")}`;
  }
  if (eventStatus === "published") {
    return "目前資格設定沒有符合員工，不能直接發布。請調整資格或先儲存草稿。";
  }
  return "目前資格設定沒有符合員工，草稿可先儲存。";
}
