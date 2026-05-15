import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import {
  archiveEvent,
  changeEventState,
  createEvent,
  duplicateEvent,
  employees,
  listAdminEvents,
  seedDemo,
  updateEvent,
} from "@/lib/api";
import type {
  CreateEventRequest,
  EventSummary,
  UpdateEventRequest,
} from "@/lib/api";
import {
  defaultEditEventForm,
  defaultEventForm,
  editFormFromEvent,
  employeeMatchesRule,
  errorMessage,
  splitTags,
  toISO,
} from "@/lib/formatting";
import { Alert, Field, Kpi, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { AdminCreateResult } from "./admin-create-result";
import { AdminEventGovernancePanel } from "./admin-governance-panel";

export function AdminEventsPage() {
  const [form, setForm] = useState(defaultEventForm);
  const [created, setCreated] = useState<EventSummary | null>(null);
  const [adminEvents, setAdminEvents] = useState<EventSummary[]>([]);
  const [selectedEventID, setSelectedEventID] = useState("");
  const [editForm, setEditForm] = useState(() => defaultEditEventForm());
  const [stateForm, setStateForm] = useState({
    status: "published",
    reason: "admin state change",
  });
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const selectedAdminEvent =
    adminEvents.find((event) => event.event_id === selectedEventID) ||
    adminEvents[0];
  const previewEmployees = useMemo(
    () =>
      employees.filter((employee) =>
        employeeMatchesRule(employee, {
          department: form.department,
          site: form.site,
          min_grade: Number(form.min_grade),
          employment_status: form.employment_status,
        }),
      ),
    [form.department, form.site, form.min_grade, form.employment_status],
  );
  const capacityReady =
    form.capacity_type === "unlimited" ||
    (Number.isFinite(Number(form.capacity)) && Number(form.capacity) > 0);
  const startsAt = new Date(form.starts_at);
  const registrationStart = new Date(form.registration_start);
  const registrationClose = new Date(form.registration_close);
  const windowReady =
    [startsAt, registrationStart, registrationClose].every(
      (date) => !Number.isNaN(date.getTime()),
    ) &&
    registrationStart <= registrationClose &&
    registrationClose <= startsAt;
  const publishChecks = [
    {
      label: "基本資料完整",
      ok: Boolean(
        form.title.trim() && form.location.trim() && form.description.trim(),
      ),
    },
    { label: "容量可用", ok: capacityReady },
    { label: "報名期間有效", ok: windowReady },
    { label: "資格規則命中", ok: previewEmployees.length > 0 },
    { label: "狀態可發布", ok: form.status === "published" },
  ];
  const canSubmit = publishChecks
    .filter((item) => item.label !== "資格規則命中")
    .every((item) => item.ok);

  async function refreshAdminEvents(nextSelectedID = selectedEventID) {
    try {
      const rows = await listAdminEvents();
      setAdminEvents(rows);
      const nextSelected = nextSelectedID || rows[0]?.event_id || "";
      setSelectedEventID(nextSelected);
      const nextEvent =
        rows.find((event) => event.event_id === nextSelected) || rows[0];
      if (nextEvent) {
        setEditForm(editFormFromEvent(nextEvent));
        setStateForm({
          status: nextEvent.status,
          reason: "admin state change",
        });
      }
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  useEffect(() => {
    void refreshAdminEvents();
  }, []);

  async function seed() {
    setBusy(true);
    setMessage("");
    try {
      const result = await seedDemo();
      setMessage(
        result.status === "seeded" ? "HR 示範員工已建立。" : result.status,
      );
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    try {
      const unlimited = form.capacity_type === "unlimited";
      const body: CreateEventRequest = {
        title: form.title.trim(),
        description: form.description.trim(),
        location: form.location.trim(),
        starts_at: toISO(form.starts_at),
        registration_start: toISO(form.registration_start),
        registration_close: toISO(form.registration_close),
        capacity_type: form.capacity_type,
        capacity: unlimited ? null : Number(form.capacity),
        allows_family: unlimited,
        status: form.status,
        rule: {
          department: form.department.trim() || "*",
          site: form.site.trim() || "*",
          min_grade: Number(form.min_grade),
          employment_status: form.employment_status.trim() || "active",
        },
      };
      const result = await createEvent(body);
      setCreated(result);
      setMessage("活動已建立並寫入 audit log。");
      await refreshAdminEvents(result.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function saveSelected(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedAdminEvent) return;
    setBusy(true);
    setMessage("");
    try {
      const unlimited = editForm.capacity_type === "unlimited";
      const body: UpdateEventRequest = {
        title: editForm.title.trim(),
        description: editForm.description.trim(),
        location: editForm.location.trim(),
        starts_at: toISO(editForm.starts_at),
        registration_start: toISO(editForm.registration_start),
        registration_close: toISO(editForm.registration_close),
        capacity_type: editForm.capacity_type,
        capacity: unlimited ? null : Number(editForm.capacity),
        allows_family: unlimited,
        category: editForm.category.trim(),
        tags: splitTags(editForm.tags),
        entry_method: editForm.entry_method.trim(),
        visibility: editForm.visibility.trim(),
      };
      const updated = await updateEvent(selectedAdminEvent.event_id, body);
      setMessage("活動已更新並寫入 audit log。");
      await refreshAdminEvents(updated.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function changeSelectedState() {
    if (!selectedAdminEvent) return;
    setBusy(true);
    setMessage("");
    try {
      const updated = await changeEventState(
        selectedAdminEvent.event_id,
        stateForm.status,
        stateForm.reason,
      );
      setMessage(`狀態已更新為 ${updated.status}。`);
      await refreshAdminEvents(updated.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function duplicateSelected() {
    if (!selectedAdminEvent) return;
    setBusy(true);
    setMessage("");
    try {
      const duplicate = await duplicateEvent(selectedAdminEvent.event_id);
      setMessage(`已複製活動：${duplicate.title}`);
      await refreshAdminEvents(duplicate.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function archiveSelected() {
    if (!selectedAdminEvent) return;
    setBusy(true);
    setMessage("");
    try {
      const archived = await archiveEvent(selectedAdminEvent.event_id);
      setMessage(`活動已封存：${archived.title}`);
      await refreshAdminEvents(archived.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="content-grid">
      <div className="panel span-12 workspace-context admin-context">
        <div>
          <div className="eyebrow">Admin Console</div>
          <h2>活動主辦入口</h2>
          <p>
            建立活動前先檢查容量、報名期間與資格命中人數；新增治理 API
            支援列表、更新、狀態、複製與封存。
          </p>
        </div>
        <div className="context-kpis">
          <Kpi label="Demo 符合人數" value={previewEmployees.length} />
          <Kpi
            label="容量"
            value={form.capacity_type === "unlimited" ? "不限" : form.capacity}
          />
          <Kpi label="管理活動" value={adminEvents.length} />
        </div>
      </div>
      <AdminEventGovernancePanel
        adminEvents={adminEvents}
        busy={busy}
        editForm={editForm}
        onArchive={() => void archiveSelected()}
        onChangeState={() => void changeSelectedState()}
        onDuplicate={() => void duplicateSelected()}
        onEditFormChange={setEditForm}
        onRefresh={() => void refreshAdminEvents()}
        onSave={saveSelected}
        onSelect={(event) => {
          setSelectedEventID(event.event_id);
          setEditForm(editFormFromEvent(event));
          setStateForm({
            status: event.status,
            reason: "admin state change",
          });
        }}
        onStateFormChange={setStateForm}
        selectedEvent={selectedAdminEvent}
        stateForm={stateForm}
      />
      <form
        className="panel span-8 form-grid"
        onSubmit={(event) => void submit(event)}
      >
        <div className="section-heading full">
          <div>
            <h2>建立活動與資格規則</h2>
            <p>Phase 1 支援先搶先得、容量、防超賣與單一資格規則。</p>
          </div>
          <button
            className="button ghost"
            type="button"
            onClick={() => void seed()}
            disabled={busy}
          >
            <Icon name="database" />
            Seed HR
          </button>
        </div>
        <fieldset className="form-section full">
          <legend>基本資料</legend>
          <Field
            label="活動名稱"
            value={form.title}
            onChange={(value) => setForm({ ...form, title: value })}
            required
          />
          <Field
            label="地點"
            value={form.location}
            onChange={(value) => setForm({ ...form, location: value })}
            required
          />
          <label className="field full">
            <span>描述</span>
            <textarea
              value={form.description}
              onChange={(event) =>
                setForm({ ...form, description: event.target.value })
              }
              rows={4}
            />
          </label>
        </fieldset>
        <fieldset className="form-section full">
          <legend>容量與報名期間</legend>
          <Field
            label="活動開始"
            type="datetime-local"
            value={form.starts_at}
            onChange={(value) => setForm({ ...form, starts_at: value })}
            required
          />
          <Field
            label="報名開始"
            type="datetime-local"
            value={form.registration_start}
            onChange={(value) =>
              setForm({ ...form, registration_start: value })
            }
            required
          />
          <Field
            label="報名截止"
            type="datetime-local"
            value={form.registration_close}
            onChange={(value) =>
              setForm({ ...form, registration_close: value })
            }
            required
          />
          <label className="field">
            <span>票數類型</span>
            <select
              value={form.capacity_type}
              onChange={(event) => {
                const next = event.target.value as "limited" | "unlimited";
                setForm({
                  ...form,
                  capacity_type: next,
                  capacity: next === "unlimited" ? "" : form.capacity || "1",
                });
              }}
            >
              <option value="limited">limited（本人單張票）</option>
              <option value="unlimited">unlimited（不扣庫存，可帶家屬）</option>
            </select>
          </label>
          {form.capacity_type === "limited" ? (
            <Field
              label="容量"
              type="number"
              value={form.capacity}
              onChange={(value) => setForm({ ...form, capacity: value })}
              required
            />
          ) : (
            <p className="form-hint full">
              unlimited 活動不設總名額，員工報名時可填寫攜帶家屬人數（上限
              10）。
            </p>
          )}
        </fieldset>
        <fieldset className="form-section full">
          <legend>資格規則</legend>
          <Field
            label="部門"
            value={form.department}
            onChange={(value) => setForm({ ...form, department: value })}
          />
          <Field
            label="廠區"
            value={form.site}
            onChange={(value) => setForm({ ...form, site: value })}
          />
          <Field
            label="最低職等"
            type="number"
            value={form.min_grade}
            onChange={(value) => setForm({ ...form, min_grade: value })}
          />
          <Field
            label="雇用狀態"
            value={form.employment_status}
            onChange={(value) => setForm({ ...form, employment_status: value })}
          />
        </fieldset>
        <div className="publish-check full" aria-live="polite">
          <StatusBadge tone={previewEmployees.length > 0 ? "ok" : "warn"}>
            {previewEmployees.length > 0 ? "資格命中" : "0 人符合"}
          </StatusBadge>
          <span>
            {previewEmployees.length > 0
              ? `Demo HR 資料中符合：${previewEmployees.map((employee) => employee.employee_id).join(", ")}`
              : "目前資格設定沒有符合的 demo 員工，發布前請確認條件。"}
          </span>
        </div>
        <div className="readiness-grid full" aria-label="發布檢查">
          {publishChecks.map((item) => (
            <div
              className={item.ok ? "readiness-item ok" : "readiness-item warn"}
              key={item.label}
            >
              <StatusBadge tone={item.ok ? "ok" : "warn"}>
                {item.ok ? "OK" : "Check"}
              </StatusBadge>
              <span>{item.label}</span>
            </div>
          ))}
        </div>
        {!windowReady && (
          <Alert tone="warn">
            報名期間需早於活動開始，且報名開始不可晚於報名截止。
          </Alert>
        )}
        <div className="form-actions full">
          <button
            className="button"
            type="submit"
            disabled={busy || !canSubmit}
          >
            <Icon name="plus" />
            建立活動
          </button>
          <button
            className="button secondary"
            type="button"
            onClick={() => setForm(defaultEventForm)}
          >
            重設
          </button>
        </div>
        {message && (
          <Alert tone={message.includes("已") ? "ok" : "warn"}>{message}</Alert>
        )}
      </form>
      <AdminCreateResult event={created} />
    </section>
  );
}
