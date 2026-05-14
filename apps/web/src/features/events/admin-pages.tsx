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
    { label: "?箸鞈?摰", ok: Boolean(form.title.trim() && form.location.trim() && form.description.trim()) },
    { label: "摰寥??舐", ok: capacityReady },
    { label: "?勗?????", ok: windowReady },
    { label: "鞈閬??賭葉", ok: previewEmployees.length > 0 },
    { label: "???澆?", ok: form.status === "published" }
  ];
  const canSubmit = publishChecks.filter((item) => item.label !== "鞈閬??賭葉").every((item) => item.ok);

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
      setMessage(result.status === "seeded" ? "HR 蝷箇??∪極撌脣遣蝡? : result.status);
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
          employment_status: form.employment_status.trim() || "active"
        }
      };
      const result = await createEvent(body);
      setCreated(result);
      setMessage("瘣餃?撌脣遣蝡蒂撖怠 audit log??);
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
        visibility: editForm.visibility.trim()
      };
      const updated = await updateEvent(selectedAdminEvent.event_id, body);
      setMessage("瘣餃?撌脫?唬蒂撖怠 audit log??);
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
      setMessage(`??歇?湔??${updated.status}?);
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
      setMessage(`撌脰?鋆賣暑??${duplicate.title}`);
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
      setMessage(`瘣餃?撌脣?摮?${archived.title}`);
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
          <h2>瘣餃?銝餉齒?亙</h2>
          <p>撱箇?瘣餃???瑼Ｘ摰寥??????鞈?賭葉鈭箸嚗憓祥??API ?舀?”??啜???鋆質?撠???/p>
        </div>
        <div className="context-kpis">
          <Kpi label="Demo 蝚血?鈭箸" value={previewEmployees.length} />
          <Kpi label="摰寥?" value={form.capacity_type === "unlimited" ? "銝?" : form.capacity} />
          <Kpi label="蝞∠?瘣餃?" value={adminEvents.length} />
        </div>
      </div>
      <div className="panel span-12">
        <div className="section-heading">
          <div>
            <h2>瘣餃?瘝餌?</h2>
            <p>蝞∠??Ｘ?瘣餃??蝺刻摩甈???????鋆質?撠?嚗??潸??耨?嫣?靽?敺垢????/p>
          </div>
          <button className="button secondary" type="button" onClick={() => void refreshAdminEvents()} disabled={busy}>
            <Icon name="refresh" />
            ??渡?
          </button>
        </div>
        <div className="governance-grid">
          <div className="event-list compact-list">
            {adminEvents.length === 0 && <EmptyState title="撠蝞∠?瘣餃?" action="撱箇?瘣餃??銵?Demo Runbook 敺??箇?券ㄐ?? />}
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
            {!selectedAdminEvent && <EmptyState title="撠?豢?瘣餃?" action="?豢?瘣餃?敺?舐楊頛?Phase 1 ?舀祥??雿? />}
            {selectedAdminEvent && (
              <>
                <div className="readiness-grid">
                  <Kpi label="Confirmed" value={selectedAdminEvent.confirmed_count} />
                  <Kpi label="Waitlist" value={selectedAdminEvent.waitlist_count} />
                  <Kpi label="?拚?" value={selectedAdminEvent.remaining_capacity ?? "No cap"} />
                  <Kpi label="Version" value={selectedAdminEvent.version || 1} />
                </div>
                <fieldset className="form-section full">
                  <legend>?舐楊頛舀?雿?/legend>
                  <Field label="瘣餃??迂" value={editForm.title} onChange={(value) => setEditForm({ ...editForm, title: value })} required />
                  <Field label="?圈?" value={editForm.location} onChange={(value) => setEditForm({ ...editForm, location: value })} required />
                  <Field
                    label="瘣餃???"
                    type="datetime-local"
                    value={editForm.starts_at}
                    onChange={(value) => setEditForm({ ...editForm, starts_at: value })}
                    required
                  />
                  <Field
                    label="?勗???"
                    type="datetime-local"
                    value={editForm.registration_start}
                    onChange={(value) => setEditForm({ ...editForm, registration_start: value })}
                    required
                  />
                  <Field
                    label="?勗??芣迫"
                    type="datetime-local"
                    value={editForm.registration_close}
                    onChange={(value) => setEditForm({ ...editForm, registration_close: value })}
                    required
                  />
                  <label className="field">
                    <span>蟡冽憿?</span>
                    <select
                      value={editForm.capacity_type}
                      onChange={(event) => {
                        const next = event.target.value as "limited" | "unlimited";
                        setEditForm({ ...editForm, capacity_type: next, capacity: next === "unlimited" ? "" : editForm.capacity || "1" });
                      }}
                    >
                      <option value="limited">limited嚗鈭箏撘萇巨嚗?/option>
                      <option value="unlimited">unlimited嚗???澈摮??臬葆摰嗅惇嚗?/option>
                    </select>
                  </label>
                  {editForm.capacity_type === "limited" && (
                    <Field label="摰寥?" type="number" value={editForm.capacity} onChange={(value) => setEditForm({ ...editForm, capacity: value })} required />
                  )}
                  <Field label="??" value={editForm.category} onChange={(value) => setEditForm({ ...editForm, category: value })} />
                  <Field label="Tags" value={editForm.tags} onChange={(value) => setEditForm({ ...editForm, tags: value })} />
                  <Field label="?亙?孵?" value={editForm.entry_method} onChange={(value) => setEditForm({ ...editForm, entry_method: value })} />
                  <Field label="?航??? value={editForm.visibility} onChange={(value) => setEditForm({ ...editForm, visibility: value })} />
                  <label className="field full">
                    <span>?膩</span>
                    <textarea
                      value={editForm.description}
                      onChange={(event) => setEditForm({ ...editForm, description: event.target.value })}
                      rows={3}
                    />
                  </label>
                </fieldset>
                <div className="state-tools full">
                  <label className="field">
                    <span>???/span>
                    <select value={stateForm.status} onChange={(event) => setStateForm({ ...stateForm, status: event.target.value })}>
                      <option value="draft">draft</option>
                      <option value="published">published</option>
                      <option value="closed">closed</option>
                      <option value="cancelled">cancelled</option>
                      <option value="archived">archived</option>
                    </select>
                  </label>
                  <Field label="????? value={stateForm.reason} onChange={(value) => setStateForm({ ...stateForm, reason: value })} required />
                  <button className="button secondary" type="button" onClick={() => void changeSelectedState()} disabled={busy || !stateForm.reason.trim()}>
                    <Icon name="save" />
                    ?湔???
                  </button>
                </div>
                <div className="form-actions full">
                  <button className="button" type="submit" disabled={busy}>
                    <Icon name="save" />
                    ?脣?瘣餃?
                  </button>
                  <button className="button secondary" type="button" onClick={() => void duplicateSelected()} disabled={busy}>
                    <Icon name="copy" />
                    銴ˊ
                  </button>
                  <button className="button danger" type="button" onClick={() => void archiveSelected()} disabled={busy}>
                    <Icon name="trash" />
                    撠?
                  </button>
                  <button className="button ghost" type="button" onClick={() => navigate("/admin/registrations")}>
                    <Icon name="users" />
                    ?勗?瘝餌?
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
            <h2>撱箇?瘣餃????潸???/h2>
            <p>Phase 1 ?舀????捆?頞都?銝鞈閬???/p>
          </div>
          <button className="button ghost" type="button" onClick={() => void seed()} disabled={busy}>
            <Icon name="database" />
            Seed HR
          </button>
        </div>
        <fieldset className="form-section full">
          <legend>?箸鞈?</legend>
          <Field label="瘣餃??迂" value={form.title} onChange={(value) => setForm({ ...form, title: value })} required />
          <Field label="?圈?" value={form.location} onChange={(value) => setForm({ ...form, location: value })} required />
          <label className="field full">
            <span>?膩</span>
            <textarea value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} rows={4} />
          </label>
        </fieldset>
        <fieldset className="form-section full">
          <legend>摰寥??????/legend>
          <Field label="瘣餃???" type="datetime-local" value={form.starts_at} onChange={(value) => setForm({ ...form, starts_at: value })} required />
          <Field
            label="?勗???"
            type="datetime-local"
            value={form.registration_start}
            onChange={(value) => setForm({ ...form, registration_start: value })}
            required
          />
          <Field
            label="?勗??芣迫"
            type="datetime-local"
            value={form.registration_close}
            onChange={(value) => setForm({ ...form, registration_close: value })}
            required
          />
          <label className="field">
            <span>蟡冽憿?</span>
            <select
              value={form.capacity_type}
              onChange={(event) => {
                const next = event.target.value as "limited" | "unlimited";
                setForm({ ...form, capacity_type: next, capacity: next === "unlimited" ? "" : form.capacity || "1" });
              }}
            >
              <option value="limited">limited嚗鈭箏撘萇巨嚗?/option>
              <option value="unlimited">unlimited嚗???澈摮??臬葆摰嗅惇嚗?/option>
            </select>
          </label>
          {form.capacity_type === "limited" ? (
            <Field label="摰寥?" type="number" value={form.capacity} onChange={(value) => setForm({ ...form, capacity: value })} required />
          ) : (
            <p className="form-hint full">unlimited 瘣餃?銝身蝮賢?憿??∪極?勗??憛怠神?葆摰嗅惇鈭箸嚗???10嚗?/p>
          )}
        </fieldset>
        <fieldset className="form-section full">
          <legend>鞈閬?</legend>
          <Field label="?券?" value={form.department} onChange={(value) => setForm({ ...form, department: value })} />
          <Field label="撱?" value={form.site} onChange={(value) => setForm({ ...form, site: value })} />
          <Field label="?雿蝑? type="number" value={form.min_grade} onChange={(value) => setForm({ ...form, min_grade: value })} />
          <Field
            label="????
            value={form.employment_status}
            onChange={(value) => setForm({ ...form, employment_status: value })}
          />
        </fieldset>
        <div className="publish-check full" aria-live="polite">
          <StatusBadge tone={previewEmployees.length > 0 ? "ok" : "warn"}>{previewEmployees.length > 0 ? "鞈?賭葉" : "0 鈭箇泵??}</StatusBadge>
          <span>
            {previewEmployees.length > 0
              ? `Demo HR 鞈?銝剔泵??${previewEmployees.map((employee) => employee.employee_id).join(", ")}`
              : "?桀?鞈閮剖?瘝?蝚血???demo ?∪極嚗撣?隢Ⅱ隤?隞嗚?}
          </span>
        </div>
        <div className="readiness-grid full" aria-label="?澆?瑼Ｘ">
          {publishChecks.map((item) => (
            <div className={item.ok ? "readiness-item ok" : "readiness-item warn"} key={item.label}>
              <StatusBadge tone={item.ok ? "ok" : "warn"}>{item.ok ? "OK" : "Check"}</StatusBadge>
              <span>{item.label}</span>
            </div>
          ))}
        </div>
        {!windowReady && <Alert tone="warn">?勗?????拇瘣餃???嚗??勗???銝??勗??芣迫??/Alert>}
        <div className="form-actions full">
          <button className="button" type="submit" disabled={busy || !canSubmit}>
            <Icon name="plus" />
            撱箇?瘣餃?
          </button>
          <button className="button secondary" type="button" onClick={() => setForm(defaultEventForm)}>
            ?身
          </button>
        </div>
        {message && <Alert tone={message.includes("撌?) ? "ok" : "warn"}>{message}</Alert>}
      </form>
      <div className="panel span-4">
        <h2>撱箇?蝯?</h2>
        {!created && <EmptyState title="撠撱箇?瘣餃?" action="?漱銵典敺?憿舐內瘣餃? ID?捆??閬??? />}
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
                <dt>蟡冽憿?</dt>
                <dd>{created.capacity_type}</dd>
              </div>
              <div>
                <dt>摰寥?</dt>
                <dd>{created.capacity_type === "unlimited" ? "銝?" : created.capacity}</dd>
              </div>
              <div>
                <dt>閬?</dt>
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
