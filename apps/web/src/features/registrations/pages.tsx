import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import {
  cancelRegistration,
  listAdminEvents,
  listRegistrations,
  promoteWaitlist,
  revokeTicket,
} from "@/lib/api";
import type { EventSummary, RegistrationDetail, Ticket } from "@/lib/api";
import { errorMessage, formatDate } from "@/lib/formatting";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  ResponsiveTable,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useUrlTab } from "@/hooks/use-url-tab";
import { Button } from "@/components/ui/button";
import {
  localizedMessage,
  registrationStatusView,
  ticketStatusView,
} from "@/lib/ui/options";
import { GovernanceActionButton } from "./governance-action-button";
import { AllocationTab } from "./allocation-tab";
import {
  initialRegistrationEventID,
  registrationAttentionCount,
  sortRegistrationRowsByAttention,
} from "./registration-priority";
import {
  GovernanceConfirmationDialog,
  type GovernanceAction,
} from "./registration-governance-dialog";

const registrationTabs = [
  ["registrations", "報名名單"],
  ["waitlist", "候補名單"],
  ["allocation", "配票抽籤"],
  ["tickets", "票券狀態"],
  ["history", "取消/撤銷紀錄"],
] as const;
type RegistrationTab = (typeof registrationTabs)[number][0];
const registrationTabValues = registrationTabs.map(([value]) => value);
export function AdminRegistrationsPage() {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [eventID, setEventID] = useState(initialRegistrationEventID);
  const [rows, setRows] = useState<RegistrationDetail[]>([]);
  const [pendingGovernanceAction, setPendingGovernanceAction] =
    useState<GovernanceAction | null>(null);
  const [pendingReason, setPendingReason] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [activeTab, setActiveTab] = useUrlTab<RegistrationTab>(
    "tab",
    registrationTabValues,
    "registrations",
  );
  const selectedEvent = events.find((event) => event.event_id === eventID);

  async function refresh(nextEventID = eventID) {
    setBusy(true);
    setMessage("");
    try {
      const nextEvents = await listAdminEvents();
      setEvents(nextEvents);
      const nextID = nextEventID || nextEvents[0]?.event_id || "";
      setEventID(nextID);
      setRows(nextID ? await listRegistrations(nextID) : []);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function runGovernance(
    action: () => Promise<string>,
    nextEventID = eventID,
  ) {
    setBusy(true);
    setMessage("");
    try {
      const nextMessage = await action();
      await refresh(nextEventID);
      setMessage((current) => current || nextMessage);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function promote() {
    if (!eventID) return;
    await runGovernance(async () =>
      localizedMessage((await promoteWaitlist(eventID)).message),
    );
  }

  async function cancel(row: RegistrationDetail, reason: string) {
    await runGovernance(async () => {
      const result = await cancelRegistration(
        row.event_id,
        row.registration_id,
        reason,
        `cancel-${row.registration_id}`,
      );
      return localizedMessage(result.message);
    }, row.event_id);
  }

  async function revoke(ticket: Ticket, reason: string) {
    await runGovernance(async () => {
      await revokeTicket(ticket.ticket_id, reason);
      return "票券已撤銷。";
    }, ticket.event_id);
  }

  async function confirmGovernanceAction() {
    if (!pendingGovernanceAction || !pendingReason.trim()) return;
    const action = pendingGovernanceAction;
    const reason = pendingReason.trim();
    await (action.kind === "revoke-ticket"
      ? revoke(action.ticket, reason)
      : cancel(action.row, reason));
    setPendingGovernanceAction(null);
    setPendingReason("");
  }

  function openGovernanceAction(action: GovernanceAction) {
    setPendingGovernanceAction(action);
    setPendingReason("");
  }

  const renderCancelRegistrationAction = (row: RegistrationDetail) =>
    renderCancellationAction(
      row,
      "cancel-registration",
      "取消",
      busy,
      openGovernanceAction,
    );
  const renderCancelWaitlistAction = (row: RegistrationDetail) =>
    renderCancellationAction(
      row,
      "cancel-waitlist",
      "取消候補",
      busy,
      openGovernanceAction,
    );
  const renderTicketAction = (row: RegistrationDetail) =>
    row.ticket ? (
      <GovernanceActionButton
        buttonLabel="撤銷票券"
        disabled={busy || row.ticket.status !== "active"}
        onClick={() =>
          row.ticket &&
          openGovernanceAction({
            kind: "revoke-ticket",
            row,
            ticket: row.ticket,
          })
        }
      />
    ) : (
      <span className="table-muted">尚無票券</span>
    );

  const prioritizedRows = sortRegistrationRowsByAttention(rows);
  const confirmedRows = prioritizedRows.filter(
    (row) => row.status === "confirmed",
  );
  const waitlistRows = prioritizedRows.filter(
    (row) => row.status === "waitlisted",
  );
  const ticketRows = prioritizedRows.filter((row) => row.ticket);
  const historyRows = prioritizedRows.filter(
    (row) => row.status === "cancelled" || row.ticket?.status === "revoked",
  );

  return (
    <section className="content-grid">
      <Tabs
        className="panel span-12 focused-tabs"
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as RegistrationTab)}
      >
        <div className="section-heading">
          <div>
            <h2>{selectedEvent?.title || "報名明細"}</h2>
            <p>清單檢視、候補提升、票券撤銷與歷史紀錄各自成頁。</p>
          </div>
          <div className="toolbar">
            <SelectField
              className="compact-field"
              label="活動"
              value={eventID}
              onChange={(value) => void refresh(value)}
              options={[
                { value: "", label: "選擇活動" },
                ...events.map((event) => ({
                  value: event.event_id,
                  label: event.title,
                })),
              ]}
            />
            <TabsList>
              {registrationTabs.map(([value, label]) => (
                <TabsTrigger key={value} value={value}>
                  {label}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
        </div>
        {message && (
          <Alert
            tone={
              message.includes("取消") || message.includes("撤銷")
                ? "warn"
                : "info"
            }
          >
            {message}
          </Alert>
        )}
        <CompactStatsBar
          items={[
            { label: "已報名", value: confirmedRows.length },
            { label: "候補", value: waitlistRows.length },
            {
              label: "需處理",
              value: registrationAttentionCount(rows),
            },
            { label: "票券", value: ticketRows.length },
          ]}
          label="報名治理摘要"
        />
        <TabsContent value="registrations">
          <RegistrationTable
            rows={confirmedRows}
            emptyTitle="尚無已報名資料"
            renderAction={renderCancelRegistrationAction}
          />
        </TabsContent>
        <TabsContent value="waitlist">
          <div className="toolbar report-toolbar">
            <Button
              type="button"
              onClick={() => void promote()}
              disabled={busy || !eventID || waitlistRows.length === 0}
            >
              <Icon name="users" />
              提升候補
            </Button>
          </div>
          <RegistrationTable
            rows={waitlistRows}
            emptyTitle="尚無候補名單"
            renderAction={renderCancelWaitlistAction}
          />
        </TabsContent>
        <TabsContent value="allocation">
          <AllocationTab
            busy={busy}
            confirmedCount={confirmedRows.length}
            event={selectedEvent}
            waitlistCount={waitlistRows.length}
            onAllocated={() => void refresh(eventID)}
          />
        </TabsContent>
        <TabsContent value="tickets">
          <RegistrationTable
            rows={ticketRows}
            emptyTitle="尚無票券資料"
            renderAction={renderTicketAction}
          />
        </TabsContent>
        <TabsContent value="history">
          <RegistrationTable
            rows={historyRows}
            emptyTitle="尚無取消或撤銷紀錄"
            renderAction={renderHistoryAction}
          />
        </TabsContent>
      </Tabs>
      <GovernanceConfirmationDialog
        action={pendingGovernanceAction}
        busy={busy}
        reason={pendingReason}
        onChangeReason={setPendingReason}
        onClose={() => {
          setPendingGovernanceAction(null);
          setPendingReason("");
        }}
        onConfirm={() => void confirmGovernanceAction()}
      />
    </section>
  );
}

function RegistrationTable({
  emptyTitle,
  renderAction,
  rows,
}: Readonly<{
  emptyTitle: string;
  renderAction: (row: RegistrationDetail) => ReactNode;
  rows: RegistrationDetail[];
}>) {
  if (rows.length === 0) {
    return (
      <EmptyState title={emptyTitle} action="切換活動或先建立報名資料。" />
    );
  }
  return (
    <ResponsiveTable label="報名治理清單">
      <thead>
        <tr>
          {["員工", "報名編號", "狀態", "票券", "建立時間", "操作"].map(
            (heading) => (
              <th key={heading}>{heading}</th>
            ),
          )}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.registration_id}>
            <td>
              {row.employee_name || row.employee_id}
              <span className="table-muted">{row.employee_id}</span>
            </td>
            <td className="mono-cell">{row.registration_id}</td>
            <td>
              {renderStatusBadge(registrationStatusView(row.status))}
              {row.cancel_reason && (
                <span className="table-muted">{row.cancel_reason}</span>
              )}
            </td>
            <td>
              {row.ticket ? (
                <>
                  {renderStatusBadge(ticketStatusView(row.ticket.status))}
                  <span className="table-muted">{row.ticket.ticket_id}</span>
                </>
              ) : (
                <span className="table-muted">尚無票券</span>
              )}
            </td>
            <td>{formatDate(row.created_at)}</td>
            <td>{renderAction(row)}</td>
          </tr>
        ))}
      </tbody>
    </ResponsiveTable>
  );
}

function renderHistoryAction() {
  return <span className="table-muted">只讀紀錄</span>;
}

function renderStatusBadge(view: ReturnType<typeof registrationStatusView>) {
  return <StatusBadge tone={view.tone}>{view.label}</StatusBadge>;
}

function renderCancellationAction(
  row: RegistrationDetail,
  kind: "cancel-registration" | "cancel-waitlist",
  buttonLabel: string,
  busy: boolean,
  onOpen: (action: GovernanceAction) => void,
) {
  return (
    <GovernanceActionButton
      buttonLabel={buttonLabel}
      disabled={busy || row.status === "cancelled"}
      onClick={() => onOpen({ kind, row })}
    />
  );
}
