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
import { Alert, CompactStatsBar } from "@/components/shared";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useUrlTab } from "@/hooks/use-url-tab";
import { localizedMessage } from "@/lib/ui/options";
import {
  AdminEventCreateTab,
  AdminEventEditTab,
  AdminEventEligibilityTab,
  AdminEventListTab,
  AdminEventStatusTab,
} from "./admin-event-crud-panels";
import { AdminEventDangerTab } from "./admin-event-danger-panel";
import {
  adminEventTabs,
  matchingEmployeesForEvent,
  type AdminEventTab,
} from "./admin-event-crud-types";
import { AdminCreateResult } from "./admin-create-result";

export function AdminEventsPage() {
  const [form, setForm] = useState(defaultEventForm);
  const [created, setCreated] = useState<EventSummary | null>(null);
  const [adminEvents, setAdminEvents] = useState<EventSummary[]>([]);
  const [selectedEventID, setSelectedEventID] = useState("");
  const [editForm, setEditForm] = useState(() => defaultEditEventForm());
  const [stateForm, setStateForm] = useState({
    status: "published",
    reason: "",
  });
  const [activeTab, setActiveTab] = useUrlTab<AdminEventTab>(
    "tab",
    adminEventTabs,
    initialTab(),
  );
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
    [form.department, form.employment_status, form.min_grade, form.site],
  );
  const selectedMatches = useMemo(
    () => matchingEmployeesForEvent(employees, selectedAdminEvent),
    [selectedAdminEvent],
  );
  const capacityReady =
    form.capacity_type === "unlimited" ||
    (Number.isFinite(Number(form.capacity)) && Number(form.capacity) > 0);
  const createWindowReady = windowReady(
    form.starts_at,
    form.registration_start,
    form.registration_close,
  );
  const editWindowReady = windowReady(
    editForm.starts_at,
    editForm.registration_start,
    editForm.registration_close,
  );
  const publishChecks = [
    {
      label: "基本資料完整",
      ok: Boolean(
        form.title.trim() && form.location.trim() && form.description.trim(),
      ),
    },
    { label: "容量可用", ok: capacityReady },
    { label: "報名期間有效", ok: createWindowReady },
    { label: "資格規則命中", ok: previewEmployees.length > 0 },
    {
      label: "建立意圖明確",
      ok: form.status === "published" || form.status === "draft",
    },
  ];
  const zeroAudiencePublishBlocked =
    form.status === "published" && previewEmployees.length === 0;
  const canSubmit =
    publishChecks
      .filter((item) => item.label !== "資格規則命中")
      .every((item) => item.ok) && !zeroAudiencePublishBlocked;

  useEffect(() => {
    void refreshAdminEvents();
  }, []);

  async function refreshAdminEvents(nextSelectedID = selectedEventID) {
    try {
      const rows = await listAdminEvents();
      setAdminEvents(rows);
      applySelectedEvent(rows, nextSelectedID || rows[0]?.event_id || "");
    } catch (error) {
      setMessage(errorMessage(error));
    }
  }

  function applySelectedEvent(rows: EventSummary[], eventID: string) {
    const nextEvent =
      rows.find((event) => event.event_id === eventID) || rows[0];
    setSelectedEventID(nextEvent?.event_id || "");
    if (!nextEvent) return;
    setEditForm(editFormFromEvent(nextEvent));
    setStateForm({
      status: nextEvent.status,
      reason: "",
    });
  }

  async function seed() {
    setBusy(true);
    setMessage("");
    try {
      const result = await seedDemo();
      setMessage(localizedMessage(result.status));
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
      const result = await createEvent(createBody(form));
      setCreated(result);
      setMessage("活動已建立並寫入稽核紀錄。");
      setActiveTab("list");
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
      const updated = await updateEvent(
        selectedAdminEvent.event_id,
        updateBody(editForm),
      );
      setMessage("活動已更新並寫入稽核紀錄。");
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
      {message && (
        <Alert tone={message.includes("已") ? "ok" : "warn"}>{message}</Alert>
      )}
      <Tabs
        className="panel span-12 focused-tabs"
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as AdminEventTab)}
      >
        <div className="section-heading">
          <div>
            <h2>活動設定工作區</h2>
            <p>每個分頁只服務一個任務，重要分頁會保存在網址查詢參數。</p>
          </div>
          <TabsList>
            <TabsTrigger value="list">活動清單</TabsTrigger>
            <TabsTrigger value="create">建立活動</TabsTrigger>
            <TabsTrigger value="edit">編輯活動</TabsTrigger>
            <TabsTrigger value="status">發布狀態</TabsTrigger>
            <TabsTrigger value="eligibility">資格預覽</TabsTrigger>
            <TabsTrigger value="danger">危險操作</TabsTrigger>
          </TabsList>
        </div>
        <CompactStatsBar
          items={[
            { label: "符合資格預覽", value: previewEmployees.length },
            { label: "管理活動", value: adminEvents.length },
          ]}
          label="活動設定摘要"
        />
        <TabsContent value="list">
          <AdminEventListTab
            events={adminEvents}
            busy={busy}
            selectedEvent={selectedAdminEvent}
            onRefresh={() => void refreshAdminEvents()}
            onSelect={(event) =>
              applySelectedEvent(adminEvents, event.event_id)
            }
            onTabChange={setActiveTab}
          />
        </TabsContent>
        <TabsContent value="create">
          <div className="create-workspace">
            <AdminEventCreateTab
              busy={busy}
              canSubmit={canSubmit}
              capacityReady={capacityReady}
              form={form}
              message=""
              previewEmployees={previewEmployees}
              publishChecks={publishChecks}
              zeroAudiencePublishBlocked={zeroAudiencePublishBlocked}
              windowReady={createWindowReady}
              onFormChange={setForm}
              onReset={() => setForm(defaultEventForm())}
              onSeed={() => void seed()}
              onSubmit={submit}
            />
            <AdminCreateResult event={created} />
          </div>
        </TabsContent>
        <TabsContent value="edit">
          <AdminEventEditTab
            busy={busy}
            editForm={editForm}
            selectedEvent={selectedAdminEvent}
            windowReady={editWindowReady}
            onEditFormChange={setEditForm}
            onSave={saveSelected}
          />
        </TabsContent>
        <TabsContent value="status">
          <AdminEventStatusTab
            busy={busy}
            selectedEvent={selectedAdminEvent}
            stateForm={stateForm}
            onChangeState={() => void changeSelectedState()}
            onStateFormChange={setStateForm}
          />
        </TabsContent>
        <TabsContent value="eligibility">
          <AdminEventEligibilityTab
            selectedEvent={selectedAdminEvent}
            matchingEmployees={selectedMatches}
          />
        </TabsContent>
        <TabsContent value="danger">
          <AdminEventDangerTab
            busy={busy}
            selectedEvent={selectedAdminEvent}
            onArchive={() => void archiveSelected()}
            onDuplicate={() => void duplicateSelected()}
          />
        </TabsContent>
      </Tabs>
    </section>
  );
}

function createBody(
  form: ReturnType<typeof defaultEventForm>,
): CreateEventRequest {
  const unlimited = form.capacity_type === "unlimited";
  return {
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
    category: form.category.trim(),
    tags: splitTags(form.tags),
    entry_method: form.entry_method.trim(),
    visibility: form.visibility.trim(),
    rule: {
      department: form.department.trim() || "*",
      site: form.site.trim() || "*",
      min_grade: Number(form.min_grade),
      employment_status: form.employment_status.trim() || "active",
    },
  };
}

function updateBody(
  editForm: ReturnType<typeof defaultEditEventForm>,
): UpdateEventRequest {
  const unlimited = editForm.capacity_type === "unlimited";
  return {
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
}

function windowReady(starts: string, start: string, close: string) {
  const startsAt = new Date(starts);
  const registrationStart = new Date(start);
  const registrationClose = new Date(close);
  return (
    [startsAt, registrationStart, registrationClose].every(
      (date) => !Number.isNaN(date.getTime()),
    ) &&
    registrationStart <= registrationClose &&
    registrationClose <= startsAt
  );
}

function initialTab(): AdminEventTab {
  if (window.location.pathname.includes("/new")) return "create";
  if (window.location.pathname.includes("/edit")) return "edit";
  if (window.location.pathname.includes("/eligibility")) return "eligibility";
  return "list";
}
