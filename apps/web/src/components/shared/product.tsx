import type { ReactNode } from "react";
import type { IconName } from "@/app/routes";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Icon } from "./icon";

type StatItem = {
  label: string;
  value: number | string;
};

type AppPageHeaderProps = {
  actions?: ReactNode;
  eyebrow: string;
  icon: IconName;
  metrics?: StatItem[];
  session: ReactNode;
  title: string;
  utilities?: ReactNode;
};

export function DebugChromeGate({
  children,
  enabled,
}: {
  children: ReactNode;
  enabled: boolean;
}) {
  if (!enabled) return null;
  return <>{children}</>;
}

export function Surface({
  as = "div",
  children,
  className = "",
  variant = "panel",
}: {
  as?: "div" | "section" | "aside";
  children: ReactNode;
  className?: string;
  variant?: "panel" | "context" | "task" | "detail" | "login" | "danger";
}) {
  const Comp = as;
  return (
    <Card asChild className={`surface surface-${variant} ${className}`.trim()}>
      <Comp>{children}</Comp>
    </Card>
  );
}

export function ActionBar({
  align = "start",
  children,
  className = "",
}: {
  align?: "start" | "end" | "between";
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={`action-bar ${align} ${className}`.trim()}>{children}</div>
  );
}

export function HelperStrip({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "info" | "ok" | "warn" | "fail";
}) {
  return <div className={`helper-strip ${tone}`}>{children}</div>;
}

export function CompactStatsBar({
  items,
  label = "摘要",
}: {
  items: StatItem[];
  label?: string;
}) {
  return (
    <dl className="compact-stats-bar" aria-label={label}>
      {items.map((item) => (
        <div className="compact-stat" key={item.label}>
          <dt>{item.label}</dt>
          <dd>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function AppPageHeader({
  actions,
  eyebrow,
  icon,
  metrics,
  session,
  title,
  utilities,
}: AppPageHeaderProps) {
  return (
    <header className="page-header app-page-header">
      <div className="page-title-block">
        <div className="page-kicker">
          <span className="page-icon" aria-hidden="true">
            <Icon name={icon} />
          </span>
          <span className="eyebrow">{eyebrow}</span>
        </div>
        <h1>{title}</h1>
      </div>
      {metrics && metrics.length > 0 && (
        <CompactStatsBar items={metrics} label={`${title}摘要`} />
      )}
      <div className="header-actions">
        {session}
        {utilities && <div className="utility-actions">{utilities}</div>}
        {actions && <div className="primary-actions">{actions}</div>}
      </div>
    </header>
  );
}

export function PageHeaderCompact(props: AppPageHeaderProps) {
  return <AppPageHeader {...props} />;
}

export function EventListItem({
  actions,
  badges,
  description,
  meta,
  title,
}: {
  actions: ReactNode;
  badges: ReactNode;
  description?: ReactNode;
  meta: ReactNode;
  title: ReactNode;
}) {
  return (
    <article className="event-list-item">
      <div className="event-list-item-main">
        <div className="event-list-item-badges">{badges}</div>
        <h3>{title}</h3>
        <p className="event-list-item-meta">{meta}</p>
        {description && (
          <p className="event-list-item-description">{description}</p>
        )}
      </div>
      <div className="event-list-item-actions">{actions}</div>
    </article>
  );
}

export function DebugToggle({
  enabled,
  onToggle,
}: {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
}) {
  return (
    <Button
      aria-pressed={enabled}
      className="debug-toggle"
      size="sm"
      type="button"
      variant="ghost"
      onClick={() => onToggle(!enabled)}
    >
      <Icon name="settings" />
      Debug
    </Button>
  );
}
