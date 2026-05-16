import type {
  InputHTMLAttributes,
  KeyboardEventHandler,
  ReactNode,
} from "react";
import { type Option, type Tone } from "@/lib/ui/options";
import { Alert as UiAlert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Empty as UiEmpty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Field as UiField,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Table } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";

export * from "./identity";
export * from "./product";

export function Field({
  autoComplete,
  className,
  id,
  inputMode,
  label,
  max,
  min,
  name,
  onKeyDown,
  placeholder,
  value,
  onChange,
  type = "text",
  required = false,
  hint,
  invalid = false,
}: {
  autoComplete?: string;
  className?: string;
  id?: string;
  inputMode?: InputHTMLAttributes<HTMLInputElement>["inputMode"];
  label: string;
  max?: InputHTMLAttributes<HTMLInputElement>["max"];
  min?: InputHTMLAttributes<HTMLInputElement>["min"];
  name?: string;
  onKeyDown?: KeyboardEventHandler<HTMLInputElement>;
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
  type?: string;
  required?: boolean;
  hint?: string;
  invalid?: boolean;
}) {
  const fieldID = id || `${label.replace(/\s+/g, "-").toLowerCase()}-field`;
  const descriptionID = `${fieldID}-hint`;
  return (
    <UiField className={className} data-invalid={invalid || undefined}>
      <FieldLabel htmlFor={fieldID}>
        {label}
        {required && <em aria-label="必填"> *</em>}
      </FieldLabel>
      <Input
        autoComplete={autoComplete}
        id={fieldID}
        inputMode={inputMode}
        max={max}
        min={min}
        name={name}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
        type={type}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        required={required}
        aria-invalid={invalid || undefined}
        aria-describedby={hint ? descriptionID : undefined}
      />
      {hint &&
        (invalid ? (
          <FieldError id={descriptionID}>{hint}</FieldError>
        ) : (
          <FieldDescription id={descriptionID}>{hint}</FieldDescription>
        ))}
    </UiField>
  );
}

export function TextareaField({
  autoComplete,
  className,
  id,
  label,
  name,
  value,
  onChange,
  required = false,
  hint,
  invalid = false,
  rows,
}: {
  autoComplete?: string;
  className?: string;
  id?: string;
  label: string;
  name?: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  hint?: string;
  invalid?: boolean;
  rows?: number;
}) {
  const fieldID = id || `${label.replace(/\s+/g, "-").toLowerCase()}-textarea`;
  const descriptionID = `${fieldID}-hint`;
  return (
    <UiField className={className} data-invalid={invalid || undefined}>
      <FieldLabel htmlFor={fieldID}>
        {label}
        {required && <em aria-label="必填"> *</em>}
      </FieldLabel>
      <Textarea
        autoComplete={autoComplete}
        id={fieldID}
        name={name}
        rows={rows}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        required={required}
        aria-invalid={invalid || undefined}
        aria-describedby={hint ? descriptionID : undefined}
      />
      {hint &&
        (invalid ? (
          <FieldError id={descriptionID}>{hint}</FieldError>
        ) : (
          <FieldDescription id={descriptionID}>{hint}</FieldDescription>
        ))}
    </UiField>
  );
}

export function SelectField({
  id,
  className,
  label,
  name,
  value,
  onChange,
  options,
  required = false,
  hint,
  invalid = false,
}: {
  id?: string;
  className?: string;
  label: string;
  name?: string;
  value: string;
  onChange: (value: string) => void;
  options: Option[];
  required?: boolean;
  hint?: string;
  invalid?: boolean;
}) {
  const emptyValue = "__empty";
  const triggerValue = value === "" ? emptyValue : value;
  const fieldID = id || `${label.replace(/\s+/g, "-").toLowerCase()}-select`;
  const descriptionID = `${fieldID}-hint`;
  return (
    <UiField className={className} data-invalid={invalid || undefined}>
      <FieldLabel htmlFor={fieldID}>
        {label}
        {required && <em aria-label="必填"> *</em>}
      </FieldLabel>
      <Select
        value={triggerValue}
        onValueChange={(next) => onChange(next === emptyValue ? "" : next)}
      >
        <SelectTrigger
          className="w-full"
          id={fieldID}
          name={name}
          aria-label={label}
          aria-invalid={invalid || undefined}
          aria-describedby={hint ? descriptionID : undefined}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map((option) => (
              <SelectItem
                key={option.value || emptyValue}
                value={option.value || emptyValue}
              >
                <span className="select-option-copy">
                  <span>{option.label}</span>
                  {option.helper && <small>{option.helper}</small>}
                </span>
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {hint &&
        (invalid ? (
          <FieldError id={descriptionID}>{hint}</FieldError>
        ) : (
          <FieldDescription id={descriptionID}>{hint}</FieldDescription>
        ))}
    </UiField>
  );
}

export function ReadinessMessage({
  label,
  message,
  tone,
}: {
  label: string;
  message: string;
  tone: Tone;
}) {
  return (
    <div className="publish-check" aria-live="polite">
      <StatusBadge tone={tone}>{label}</StatusBadge>
      <span>{message}</span>
    </div>
  );
}

export function SegmentedFilter({
  label,
  value,
  onChange,
  options,
  counts = {},
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: Option[];
  counts?: Record<string, number>;
}) {
  return (
    <div className="segmented-filter" role="group" aria-label={label}>
      {options.map((option) => (
        <Button
          aria-pressed={value === option.value}
          className={
            value === option.value
              ? "segmented-filter-item active"
              : "segmented-filter-item"
          }
          key={option.value}
          onClick={() => onChange(option.value)}
          size="sm"
          type="button"
          variant={value === option.value ? "secondary" : "ghost"}
        >
          <span>{option.label}</span>
          {counts[option.value] !== undefined && (
            <strong>{counts[option.value]}</strong>
          )}
        </Button>
      ))}
    </div>
  );
}

export function Alert({
  children,
  tone,
}: {
  children: ReactNode;
  tone: Exclude<Tone, "neutral">;
}) {
  const urgent = tone === "fail" || tone === "warn";
  return (
    <UiAlert
      className={`app-alert ${tone}`}
      role={urgent ? "alert" : "status"}
      aria-live={urgent ? "assertive" : "polite"}
    >
      <AlertDescription>{children}</AlertDescription>
    </UiAlert>
  );
}

export function StatusBadge({
  children,
  tone,
}: {
  children: ReactNode;
  tone: Tone;
}) {
  return (
    <Badge
      className={`status-badge ${tone}`}
      data-tone={tone}
      variant="outline"
    >
      {children}
    </Badge>
  );
}

export function Kpi({
  label,
  value,
}: {
  label: string;
  value: number | string;
}) {
  return (
    <Card className="kpi" size="sm">
      <strong>{value}</strong>
      <span>{label}</span>
    </Card>
  );
}

export function ProgressMeter({
  label,
  value,
  max,
  helper,
  compact = false,
}: {
  label: string;
  value: number;
  max: number;
  helper: string;
  compact?: boolean;
}) {
  const ratio = max > 0 ? Math.min(Math.max(value / max, 0), 1) : 0;
  return (
    <div
      className={compact ? "progress-meter compact" : "progress-meter"}
      aria-label={`${label} ${helper}`}
      aria-valuemax={max}
      aria-valuemin={0}
      aria-valuenow={Math.min(Math.max(value, 0), max)}
      role="progressbar"
    >
      <div className="progress-meter-head">
        <span>{label}</span>
        <strong>{helper}</strong>
      </div>
      <div className="progress-track">
        <span style={{ width: `${Math.round(ratio * 100)}%` }} />
      </div>
    </div>
  );
}

export function EmptyState({
  title,
  action,
}: {
  title: string;
  action: string;
}) {
  return (
    <UiEmpty className="empty-state">
      <EmptyHeader>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{action}</EmptyDescription>
      </EmptyHeader>
    </UiEmpty>
  );
}

export function DangerZonePanel({ children }: { children: ReactNode }) {
  return <Card className="danger-zone-panel">{children}</Card>;
}

export function ResponsiveTable({
  children,
  label = "資料表",
  mobileCards,
}: {
  children: ReactNode;
  label?: string;
  mobileCards?: ReactNode;
}) {
  return (
    <div
      className={
        mobileCards
          ? "responsive-table-stack has-mobile"
          : "responsive-table-stack"
      }
    >
      <div
        className="table-scroll"
        data-slot="responsive-table"
        role="region"
        aria-label={label}
        tabIndex={0}
      >
        <Table>
          <caption className="sr-only">{label}</caption>
          {children}
        </Table>
      </div>
      {mobileCards && (
        <div className="mobile-table-cards" aria-label={`${label}行動版清單`}>
          {mobileCards}
        </div>
      )}
    </div>
  );
}

export function SkeletonRows({ rows }: { rows: number }) {
  return (
    <div className="skeleton-stack" role="status" aria-live="polite">
      <span className="sr-only">載入中…</span>
      {Array.from({ length: rows }, (_, index) => (
        <Skeleton className="skeleton-row" key={index} />
      ))}
    </div>
  );
}
