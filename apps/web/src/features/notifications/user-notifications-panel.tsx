import { navigate } from "@/app/routes";
import {
  Alert,
  CompactStatsBar,
  EmptyState,
  ResponsiveTable,
  SkeletonRows,
  StatusBadge,
} from "@/components/shared";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { runClientNavigation } from "@/lib/navigation";
import { registrationStatusView, ticketStatusView } from "@/lib/ui/options";

export type UserNotificationRow = {
  id: string;
  href: string;
  kind: string;
  title: string;
  status?: string;
  detail: string;
};

export function UserNotificationsPanel({
  loading,
  message,
  notificationRows,
  onRefresh,
  ticketCount,
  waitlistedCount,
}: Readonly<{
  loading: boolean;
  message: string;
  notificationRows: UserNotificationRow[];
  onRefresh: () => void;
  ticketCount: number;
  waitlistedCount: number;
}>) {
  return (
    <section className="content-grid">
      <Card className="panel span-12" aria-busy={loading}>
        <div className="section-heading">
          <div>
            <h2>通知清單</h2>
            <p>報名、候補、票券與入場相關狀態集中在同一張表。</p>
          </div>
        </div>
        <CompactStatsBar
          items={[
            { label: "通知", value: notificationRows.length },
            { label: "候補更新", value: waitlistedCount },
            { label: "票券更新", value: ticketCount },
          ]}
          label="通知摘要"
        />
        {message && (
          <Alert tone="fail">
            {message}
            <Button
              variant="outline"
              size="sm"
              type="button"
              onClick={onRefresh}
              disabled={loading}
            >
              重新整理
            </Button>
          </Alert>
        )}
        {loading ? (
          <SkeletonRows rows={4} />
        ) : (
          <ResponsiveTable
            label="通知清單"
            mobileCards={notificationRows.map((row) => (
              <article className="mobile-summary-card" key={row.id}>
                <div>
                  <h3>
                    <NotificationSubject href={row.href} title={row.title} />
                  </h3>
                  <p className="table-muted">{row.kind}</p>
                </div>
                <NotificationStatusBadge kind={row.kind} status={row.status} />
                <p>{row.detail}</p>
              </article>
            ))}
          >
            <thead>
              <tr>
                <th scope="col">類型</th>
                <th scope="col">主旨</th>
                <th scope="col">狀態</th>
                <th scope="col">說明</th>
              </tr>
            </thead>
            <tbody>
              {notificationRows.map((row) => (
                <tr key={row.id}>
                  <td>{row.kind}</td>
                  <td>
                    <NotificationSubject href={row.href} title={row.title} />
                  </td>
                  <td>
                    <NotificationStatusBadge
                      kind={row.kind}
                      status={row.status}
                    />
                  </td>
                  <td>{row.detail}</td>
                </tr>
              ))}
            </tbody>
          </ResponsiveTable>
        )}
        {!loading && !message && notificationRows.length === 0 && (
          <EmptyState
            title="尚無通知"
            action="報名、候補或票券狀態有變更時，通知會出現在這裡。"
          />
        )}
      </Card>
    </section>
  );
}

function NotificationSubject({
  href,
  title,
}: Readonly<{ href: string; title: string }>) {
  return (
    <a
      href={href}
      onClick={(event) => runClientNavigation(event, () => navigate(href))}
    >
      {title}
    </a>
  );
}

function NotificationStatusBadge({
  kind,
  status = "",
}: Readonly<{ kind: string; status?: string }>) {
  const view =
    kind === "票券" ? ticketStatusView(status) : registrationStatusView(status);
  return <StatusBadge tone={view.tone}>{view.label}</StatusBadge>;
}
