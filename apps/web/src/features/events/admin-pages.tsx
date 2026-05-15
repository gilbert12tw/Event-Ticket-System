import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import { archiveEvent, changeEventState, createEvent, duplicateEvent, employees, listAdminEvents, seedDemo, updateEvent } from "@/lib/api";
import type { CreateEventRequest, EventSummary, UpdateEventRequest } from "@/lib/api";
import { navigate } from "@/app/routes";
import { defaultEditEventForm, editFormFromEvent, employeeMatchesRule, errorMessage, eventStatusTone, splitTags, toISO } from "@/lib/formatting";
import { defaultEventForm } from "@/lib/formatting";
import { Alert, EmptyState, Field, Kpi, StatusBadge } from "@/components/shared";
import { Icon } from "@/components/shared/icon";

export function AdminEventsPage() {
  const [form, setForm] = useState(defaultEventForm);
  const [created, setCreated] = useState<EventSummary | null>(null);
  const [adminEvents, setAdminEvents] = useState<EventSummary[]>([]);
  const [selectedEventID, setSelectedEventID] = useState("");
  const [editForm, setEditForm] = useState(() => defaultEditEventForm());
  const [stateForm, setStateForm] = useState({ status: "published", reason: "admin state change" });
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const selectedAdminEvent = adminEvents.find((event) => event.event_id === selectedEventID) || adminEvents[0];
  const previewEmployees = useMemo(
    () =>
      employees.filter((employee) =>
        employeeMatchesRule(employee, {
          department: form.department,
          site: form.site,
          min_grade: Number(form.min_grade),
          employment_status: form.employment_status
        })
      ),
    [form.department, form.site, form.min_grade, form.employment_status]
  );
  const capacityReady = form.capacity_type === "unlimited" || (Number.isFinite(Number(form.capacity)) && Number(form.capacity) > 0);
  const startsAt = new Date(form.starts_at);
  const registrationStart = new Date(form.registration_start);
  const registrationClose = new Date(form.registration_close);
  const windowReady =
    [startsAt, registrationStart, registrationClose].every((date) => !Number.isNaN(date.getTime())) &&
    registrationStart <= registrationClose &&
    registrationClose <= startsAt;
  const publishChecks = [
    { label: "基本資料完整", ok: Boolean(form.title.trim() && form.location.trim() && form.description.trim()) },
    { label: "容量可用", ok: capacityReady },
    { label: "報名期間有效", ok: windowReady },
    { label: "資格規則命中", ok: previewEmployees.length > 0 },
    { label: "狀態可發布", ok: form.status === "published" }
  ];
  const canSubmit = publishChecks.filter((item) => item.label !== "資格規則命中").every((item) => item.ok);

  async function refreshAdminEvents(nextSelectedID = selectedEventID) {
    try {
      const rows = await listAdminEvents();
      setAdminEvents(rows);
      const nextSelected = nextSelectedID || rows[0]?.event_id || "";
      setSelectedEventID(nextSelected);
      const nextEvent = rows.find((event) => event.event_id === nextSelected) || rows[0];
      if (nextEvent) {
        setEditForm(editFormFromEvent(nextEvent));
        setStateForm({ status: nextEvent.status, reason: "admin state change" });
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
      setMessage(result.status === "seeded" ? "HR 示範員工已建立。" : result.status);
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
        event_city: form.event_city.trim() || undefined,
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
          employment_status: form.employment_status.trim() || "active"
        }
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
        event_city: editForm.event_city.trim() || undefined,
        starts_at: toISO(editForm.starts_at),
        registration_start: toISO(editForm.registration_start),
        registration_close: toISO(editForm.registration_close),
        capacity_type: editForm.capacity_type,
        capacity: unlimited ? null : Number(editForm.capacity),
        allows_family: unlimited,
        category: editForm.category.trim(),
        tags: splitTags(editForm.tags),
        entry_method: editForm.entry_method.trim(),
        visibility: editForm.visibility.trim()
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
      const updated = await changeEventState(selectedAdminEvent.event_id, stateForm.status, stateForm.reason);
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
          <p>建立活動前先檢查容量、報名期間與資格命中人數；新增治理 API 支援列表、更新、狀態、複製與封存。</p>
        </div>
        <div className="context-kpis">
          <Kpi label="Demo 符合人數" value={previewEmployees.length} />
          <Kpi label="容量" value={form.capacity_type === "unlimited" ? "不限" : form.capacity} />
          <Kpi label="管理活動" value={adminEvents.length} />
        </div>
      </div>
      <div className="panel span-12">
        <div className="section-heading">
          <div>
            <h2>活動治理</h2>
            <p>管理既有活動的可編輯欄位、狀態轉換、複製與封存，資格規則修改仍保留後端邊界。</p>
          </div>
          <button className="button secondary" type="button" onClick={() => void refreshAdminEvents()} disabled={busy}>
            <Icon name="refresh" />
            重新整理
          </button>
        </div>
        <div className="governance-grid">
          <div className="event-list compact-list">
            {adminEvents.length === 0 && <EmptyState title="尚無管理活動" action="建立活動或執行 Demo Runbook 後會出現在這裡。" />}
            {adminEvents.map((event) => (
              <button
                className={selectedAdminEvent?.event_id === event.event_id ? "event-row active" : "event-row"}
                type="button"
                key={event.event_id}
                onClick={() => {
                  setSelectedEventID(event.event_id);
                  setEditForm(editFormFromEvent(event));
                  setStateForm({ status: event.status, reason: "admin state change" });
                }}
              >
                <span>
                  <strong>{event.title}</strong>
                  <small>{event.event_id}</small>
                </span>
                <StatusBadge tone={eventStatusTone(event.status)}>{event.status}</StatusBadge>
              </button>
            ))}
          </div>
          <form className="governance-editor" onSubmit={(event) => void saveSelected(event)}>
            {!selectedAdminEvent && <EmptyState title="尚未選擇活動" action="選擇活動後即可編輯 Phase 1 可治理欄位。" />}
            {selectedAdminEvent && (
              <>
                <div className="readiness-grid">
                  <Kpi label="Confirmed" value={selectedAdminEvent.confirmed_count} />
                  <Kpi label="Waitlist" value={selectedAdminEvent.waitlist_count} />
                  <Kpi label="剩餘" value={selectedAdminEvent.capacity_type === "unlimited" ? "不限" : (selectedAdminEvent.remaining_capacity ?? 0)} />
                  <Kpi label="Version" value={selectedAdminEvent.version || 1} />
                </div>
                <fieldset className="form-section full">
                  <legend>可編輯欄位</legend>
                  <Field label="活動名稱" value={editForm.title} onChange={(value) => setEditForm({ ...editForm, title: value })} required />
                  <Field label="地點" value={editForm.location} onChange={(value) => setEditForm({ ...editForm, location: value })} required />
                  <label className="field">
                    <span>活動城市</span>
                    <select value={editForm.event_city} onChange={(event) => setEditForm({ ...editForm, event_city: event.target.value })}>
                      <option value="">（未設定）</option>
                      <option value="Taipei">台北 (Taipei)</option>
                      <option value="Hsinchu">新竹 (Hsinchu)</option>
                      <option value="Taichung">台中 (Taichung)</option>
                      <option value="Tainan">台南 (Tainan)</option>
                      <option value="Kaohsiung">高雄 (Kaohsiung)</option>
                    </select>
                    <small className="form-hint">設定後用於比對員工所在城市，不同城市將顯示跨城市提示（不阻擋報名）。</small>
                  </label>
                  <Field
                    label="活動開始"
                    type="datetime-local"
                    value={editForm.starts_at}
                    onChange={(value) => setEditForm({ ...editForm, starts_at: value })}
                    required
                  />
                  <Field
                    label="報名開始"
                    type="datetime-local"
                    value={editForm.registration_start}
                    onChange={(value) => setEditForm({ ...editForm, registration_start: value })}
                    required
                  />
                  <Field
                    label="報名截止"
                    type="datetime-local"
                    value={editForm.registration_close}
                    onChange={(value) => setEditForm({ ...editForm, registration_close: value })}
                    required
                  />
                  <label className="field">
                    <span>票數類型</span>
                    <select
                      value={editForm.capacity_type}
                      onChange={(event) => {
                        const next = event.target.value as "limited" | "unlimited";
                        setEditForm({ ...editForm, capacity_type: next, capacity: next === "unlimited" ? "" : editForm.capacity || "1" });
                      }}
                    >
                      <option value="limited">limited（本人單張票）</option>
                      <option value="unlimited">unlimited（不扣庫存，可帶家屬）</option>
                    </select>
                  </label>
                  {editForm.capacity_type === "limited" && (
                    <Field label="容量" type="number" value={editForm.capacity} onChange={(value) => setEditForm({ ...editForm, capacity: value })} required />
                  )}
                  <Field label="分類" value={editForm.category} onChange={(value) => setEditForm({ ...editForm, category: value })} />
                  <Field label="Tags" value={editForm.tags} onChange={(value) => setEditForm({ ...editForm, tags: value })} />
                  <Field label="入場方式" value={editForm.entry_method} onChange={(value) => setEditForm({ ...editForm, entry_method: value })} />
                  <Field label="可見性" value={editForm.visibility} onChange={(value) => setEditForm({ ...editForm, visibility: value })} />
                  <label className="field full">
                    <span>描述</span>
                    <textarea
                      value={editForm.description}
                      onChange={(event) => setEditForm({ ...editForm, description: event.target.value })}
                      rows={3}
                    />
                  </label>
                </fieldset>
                <div className="state-tools full">
                  <label className="field">
                    <span>狀態</span>
                    <select value={stateForm.status} onChange={(event) => setStateForm({ ...stateForm, status: event.target.value })}>
                      <option value="draft">draft</option>
                      <option value="published">published</option>
                      <option value="closed">closed</option>
                      <option value="cancelled">cancelled</option>
                      <option value="archived">archived</option>
                    </select>
                  </label>
                  <Field label="狀態原因" value={stateForm.reason} onChange={(value) => setStateForm({ ...stateForm, reason: value })} required />
                  <button className="button secondary" type="button" onClick={() => void changeSelectedState()} disabled={busy || !stateForm.reason.trim()}>
                    <Icon name="save" />
                    更新狀態
                  </button>
                </div>
                <div className="form-actions full">
                  <button className="button" type="submit" disabled={busy}>
                    <Icon name="save" />
                    儲存活動
                  </button>
                  <button className="button secondary" type="button" onClick={() => void duplicateSelected()} disabled={busy}>
                    <Icon name="copy" />
                    複製
                  </button>
                  <button className="button danger" type="button" onClick={() => void archiveSelected()} disabled={busy}>
                    <Icon name="trash" />
                    封存
                  </button>
                  <button className="button ghost" type="button" onClick={() => navigate("/admin/registrations")}>
                    <Icon name="users" />
                    報名治理
                  </button>
                </div>
              </>
            )}
          </form>
        </div>
      </div>
      <form className="panel span-8 form-grid" onSubmit={(event) => void submit(event)}>
        <div className="section-heading full">
          <div>
            <h2>建立活動與資格規則</h2>
            <p>Phase 1 支援先搶先得、容量、防超賣與單一資格規則。</p>
          </div>
          <button className="button ghost" type="button" onClick={() => void seed()} disabled={busy}>
            <Icon name="database" />
            Seed HR
          </button>
        </div>
        <fieldset className="form-section full">
          <legend>基本資料</legend>
          <Field label="活動名稱" value={form.title} onChange={(value) => setForm({ ...form, title: value })} required />
          <Field label="地點" value={form.location} onChange={(value) => setForm({ ...form, location: value })} required />
          <label className="field">
            <span>活動城市</span>
            <select value={form.event_city} onChange={(event) => setForm({ ...form, event_city: event.target.value })}>
              <option value="">（未設定）</option>
              <option value="Taipei">台北 (Taipei)</option>
              <option value="Hsinchu">新竹 (Hsinchu)</option>
              <option value="Taichung">台中 (Taichung)</option>
              <option value="Tainan">台南 (Tainan)</option>
              <option value="Kaohsiung">高雄 (Kaohsiung)</option>
            </select>
            <small className="form-hint">設定後用於比對員工所在城市，不同城市將顯示跨城市提示（不阻擋報名）。</small>
          </label>
          <label className="field full">
            <span>描述</span>
            <textarea value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} rows={4} />
          </label>
        </fieldset>
        <fieldset className="form-section full">
          <legend>容量與報名期間</legend>
          <Field label="活動開始" type="datetime-local" value={form.starts_at} onChange={(value) => setForm({ ...form, starts_at: value })} required />
          <Field
            label="報名開始"
            type="datetime-local"
            value={form.registration_start}
            onChange={(value) => setForm({ ...form, registration_start: value })}
            required
          />
          <Field
            label="報名截止"
            type="datetime-local"
            value={form.registration_close}
            onChange={(value) => setForm({ ...form, registration_close: value })}
            required
          />
          <label className="field">
            <span>票數類型</span>
            <select
              value={form.capacity_type}
              onChange={(event) => {
                const next = event.target.value as "limited" | "unlimited";
                setForm({ ...form, capacity_type: next, capacity: next === "unlimited" ? "" : form.capacity || "1" });
              }}
            >
              <option value="limited">limited（本人單張票）</option>
              <option value="unlimited">unlimited（不扣庫存，可帶家屬）</option>
            </select>
          </label>
          {form.capacity_type === "limited" ? (
            <Field label="容量" type="number" value={form.capacity} onChange={(value) => setForm({ ...form, capacity: value })} required />
          ) : (
            <p className="form-hint full">unlimited 活動不設總名額，員工報名時可填寫攜帶家屬人數（上限 10）。</p>
          )}
        </fieldset>
        <fieldset className="form-section full">
          <legend>資格規則</legend>
          <Field label="部門" value={form.department} onChange={(value) => setForm({ ...form, department: value })} />
          <Field label="廠區" value={form.site} onChange={(value) => setForm({ ...form, site: value })} />
          <Field label="最低職等" type="number" value={form.min_grade} onChange={(value) => setForm({ ...form, min_grade: value })} />
          <Field
            label="雇用狀態"
            value={form.employment_status}
            onChange={(value) => setForm({ ...form, employment_status: value })}
          />
        </fieldset>
        <div className="publish-check full" aria-live="polite">
          <StatusBadge tone={previewEmployees.length > 0 ? "ok" : "warn"}>{previewEmployees.length > 0 ? "資格命中" : "0 人符合"}</StatusBadge>
          <span>
            {previewEmployees.length > 0
              ? `Demo HR 資料中符合：${previewEmployees.map((employee) => employee.employee_id).join(", ")}`
              : "目前資格設定沒有符合的 demo 員工，發布前請確認條件。"}
          </span>
        </div>
        <div className="readiness-grid full" aria-label="發布檢查">
          {publishChecks.map((item) => (
            <div className={item.ok ? "readiness-item ok" : "readiness-item warn"} key={item.label}>
              <StatusBadge tone={item.ok ? "ok" : "warn"}>{item.ok ? "OK" : "Check"}</StatusBadge>
              <span>{item.label}</span>
            </div>
          ))}
        </div>
        {!windowReady && <Alert tone="warn">報名期間需早於活動開始，且報名開始不可晚於報名截止。</Alert>}
        <div className="form-actions full">
          <button className="button" type="submit" disabled={busy || !canSubmit}>
            <Icon name="plus" />
            建立活動
          </button>
          <button className="button secondary" type="button" onClick={() => setForm(defaultEventForm)}>
            重設
          </button>
        </div>
        {message && <Alert tone={message.includes("已") ? "ok" : "warn"}>{message}</Alert>}
      </form>
      <div className="panel span-4">
        <h2>建立結果</h2>
        {!created && <EmptyState title="尚未建立活動" action="提交表單後會顯示活動 ID、容量與規則。" />}
        {created && (
          <div className="summary-block">
            <StatusBadge tone="ok">{created.status}</StatusBadge>
            <h3>{created.title}</h3>
            <dl className="meta-list vertical">
              <div>
                <dt>Event ID</dt>
                <dd>{created.event_id}</dd>
              </div>
              <div>
                <dt>票數類型</dt>
                <dd>{created.capacity_type}</dd>
              </div>
              <div>
                <dt>容量</dt>
                <dd>{created.capacity_type === "unlimited" ? "不限" : created.capacity}</dd>
              </div>
              <div>
                <dt>規則</dt>
                <dd>
                  {created.rule.department} / {created.rule.site} / G{created.rule.min_grade}+
                </dd>
              </div>
            </dl>
          </div>
        )}
      </div>
    </section>
  );
}
