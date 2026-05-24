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
  MetaList,
  type MetaListRow,
  ResponsiveTable,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Icon } from "@/components/shared/icon";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useUrlTab } from "@/hooks/use-url-tab";
import { Button } from "@/components/ui/button";
import {
  cancellationReasonOptions,
  localizedMessage,
  registrationStatusView,
  revocationReasonOptions,
  ticketStatusView,
} from "@/lib/ui/options";
import { GovernanceActionButton } from "./governance-action-button";
import { AllocationTab } from "./allocation-tab";

const registrationTabs = [
  ["registrations", "報名名單"],
  ["waitlist", "候補名單"],
  ["allocation", "配票抽籤"],
  ["tickets", "票券狀態"],
  ["history", "取消/撤銷紀錄"],
] as const;
type RegistrationTab = (typeof registrationTabs)[number][0];
const registrationTabValues = registrationTabs.map(([value]) => value);
type GovernanceAction =
  | { kind: "cancel-registration" | "cancel-waitlist"; row: RegistrationDetail }
  | { kind: "revoke-ticket"; row: RegistrationDetail; ticket: Ticket };
const governanceActionCopies = {
  "revoke-ticket": {
    title: "確認撤銷票券",
    buttonLabel: "確認撤銷",
    consequence:
      "票券撤銷後不可入場，驗票端會改為拒絕，員工需要由主辦重新處理。",
  },
  "cancel-waitlist": {
    title: "確認取消候補",
    buttonLabel: "確認取消候補",
    consequence: "候補取消後會離開候補名單，不會再自動遞補名額。",
  },
  "cancel-registration": {
    title: "確認取消報名",
    buttonLabel: "確認取消報名",
    consequence: "報名取消後會釋出名額；若已有票券，票券治理需同步確認。",
  },
};

export function AdminRegistrationsPage() {
  const [events, setEvents] = useState<EventSummary[]>([]);
  const [eventID, setEventID] = useState("");
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

  const renderCancellationAction =
    (kind: "cancel-registration" | "cancel-waitlist", buttonLabel: string) =>
    (row: RegistrationDetail) => (
      <GovernanceActionButton
        buttonLabel={buttonLabel}
        disabled={busy || row.status === "cancelled"}
        onClick={() => openGovernanceAction({ kind, row })}
      />
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

  const confirmedRows = rows.filter((row) => row.status === "confirmed");
  const waitlistRows = rows.filter((row) => row.status === "waitlisted");
  const ticketRows = rows.filter((row) => row.ticket);
  const historyRows = rows.filter(
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
              label: "已取消",
              value: rows.filter((row) => row.status === "cancelled").length,
            },
            { label: "票券", value: ticketRows.length },
          ]}
          label="報名治理摘要"
        />
        <TabsContent value="registrations">
          <RegistrationTable
            rows={confirmedRows}
            emptyTitle="尚無已報名資料"
            renderAction={renderCancellationAction(
              "cancel-registration",
              "取消",
            )}
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
            renderAction={renderCancellationAction(
              "cancel-waitlist",
              "取消候補",
            )}
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
            renderAction={() => <span className="table-muted">只讀紀錄</span>}
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
}: {
  emptyTitle: string;
  renderAction: (row: RegistrationDetail) => ReactNode;
  rows: RegistrationDetail[];
}) {
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

function renderStatusBadge(view: ReturnType<typeof registrationStatusView>) {
  return <StatusBadge tone={view.tone}>{view.label}</StatusBadge>;
}

function GovernanceConfirmationDialog({
  action,
  busy,
  onChangeReason,
  onClose,
  onConfirm,
  reason,
}: {
  action: GovernanceAction | null;
  busy: boolean;
  onChangeReason: (reason: string) => void;
  onClose: () => void;
  onConfirm: () => void;
  reason: string;
}) {
  const copy = action ? governanceActionCopies[action.kind] : null;
  const options =
    action?.kind === "revoke-ticket"
      ? revocationReasonOptions
      : cancellationReasonOptions;
  const details: MetaListRow[] = action
    ? [
        [
          "員工",
          <>
            {action.row.employee_name || action.row.employee_id}
            <span className="table-muted">{action.row.employee_id}</span>
          </>,
        ],
        ["報名編號", action.row.registration_id],
        ...(action.kind === "revoke-ticket"
          ? ([["票券編號", action.ticket.ticket_id]] satisfies MetaListRow[])
          : []),
      ]
    : [];

  return (
    <Dialog open={Boolean(action)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{copy?.title || "確認操作"}</DialogTitle>
          <DialogDescription>
            送出後會立即影響報名或票券狀態，並寫入稽核紀錄。
          </DialogDescription>
        </DialogHeader>
        {action && copy && (
          <>
            <MetaList rows={details} />
            <Alert tone="warn">{copy.consequence}</Alert>
            <SelectField
              label="處置原因"
              value={reason}
              options={[{ value: "", label: "請選擇原因" }, ...options]}
              onChange={onChangeReason}
              required
              invalid={!reason.trim()}
              hint="必須選擇明確原因，不能用預設原因直接送出。"
            />
          </>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            type="button"
            onClick={onClose}
            disabled={busy}
          >
            返回
          </Button>
          <Button
            variant="destructive"
            type="button"
            onClick={onConfirm}
            disabled={busy || !reason.trim()}
          >
            {busy ? "處理中" : copy?.buttonLabel || "確認"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
