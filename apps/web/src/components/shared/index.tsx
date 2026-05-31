import type { InputHTMLAttributes, ReactNode } from "react";
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
export * from "./meta-list";
export * from "./product";

type BaseFieldProps = {
  className?: string;
  id?: string;
  label: string;
  name?: string;
  required?: boolean;
  hint?: string;
  invalid?: boolean;
};

type ValueFieldProps = BaseFieldProps & {
  value: string;
  onChange: (value: string) => void;
};

type ChildrenProps = { children: ReactNode };
type EmptyStateProps = { title: string; action: string };
type LabelValueProps = { label: string; value: number | string };
type ReadinessMessageProps = { label: string; message: string; tone: Tone };
type ToneChildrenProps = ChildrenProps & { tone: Tone };

type FieldProps = ValueFieldProps &
  Pick<
    InputHTMLAttributes<HTMLInputElement>,
    "autoComplete" | "inputMode" | "max" | "min" | "onKeyDown" | "placeholder"
  > & { type?: string };

type TextareaFieldProps = ValueFieldProps & {
  autoComplete?: string;
  rows?: number;
};

type SelectFieldProps = ValueFieldProps & { options: Option[] };

type ProgressMeterProps = {
  label: string;
  value: number;
  max: number;
  helper: string;
  compact?: boolean;
};

type ResponsiveTableProps = ChildrenProps & {
  label?: string;
  mobileCards?: ReactNode;
};

type ControlFieldProps = BaseFieldProps & {
  suffix: string;
  children: (
    fieldID: string,
    controlName: string,
    descriptionID: string | undefined,
  ) => ReactNode;
};

function ControlField({
  children,
  className,
  hint,
  id,
  invalid = false,
  label,
  name,
  required = false,
  suffix,
}: ControlFieldProps) {
  const fieldID = id || `${slugify(label)}-${suffix}`;
  const descriptionID = hint ? `${fieldID}-hint` : undefined;
  return (
    <UiField className={className} data-invalid={invalid || undefined}>
      <FieldLabel htmlFor={fieldID}>
        {label}
        {required && <em aria-label="必填"> *</em>}
      </FieldLabel>
      {children(fieldID, name ?? fieldID, descriptionID)}
      {hint &&
        (invalid ? (
          <FieldError id={descriptionID}>{hint}</FieldError>
        ) : (
          <FieldDescription id={descriptionID}>{hint}</FieldDescription>
        ))}
    </UiField>
  );
}

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
}: FieldProps) {
  const fieldProps = { className, hint, id, invalid, label, name, required };
  return (
    <ControlField {...fieldProps} suffix="field">
      {(fieldID, controlName, descriptionID) => (
        <Input
          autoComplete={autoComplete ?? "off"}
          id={fieldID}
          inputMode={inputMode}
          max={max}
          min={min}
          name={controlName}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          type={type}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          required={required}
          aria-invalid={invalid || undefined}
          aria-describedby={descriptionID}
        />
      )}
    </ControlField>
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
}: TextareaFieldProps) {
  const fieldProps = { className, hint, id, invalid, label, name, required };
  return (
    <ControlField {...fieldProps} suffix="textarea">
      {(fieldID, controlName, descriptionID) => (
        <Textarea
          autoComplete={autoComplete ?? "off"}
          id={fieldID}
          name={controlName}
          rows={rows}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          required={required}
          aria-invalid={invalid || undefined}
          aria-describedby={descriptionID}
        />
      )}
    </ControlField>
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
}: SelectFieldProps) {
  const fieldProps = { className, hint, id, invalid, label, name, required };
  const emptyValue = "__empty";
  const triggerValue = value === "" ? emptyValue : value;
  const selectedOption = options.find(
    (option) => (option.value || emptyValue) === triggerValue,
  );
  return (
    <ControlField {...fieldProps} suffix="select">
      {(fieldID, controlName, descriptionID) => (
        <Select
          name={controlName}
          required={required}
          value={triggerValue}
          onValueChange={(next) => onChange(next === emptyValue ? "" : next)}
        >
          <SelectTrigger
            className="w-full"
            id={fieldID}
            name={controlName}
            aria-label={label}
            aria-invalid={invalid || undefined}
            aria-describedby={descriptionID}
          >
            <SelectValue placeholder={selectedOption?.label || label} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {options.map((option) => (
                <SelectItem
                  key={option.value || emptyValue}
                  helper={option.helper}
                  textValue={option.label}
                  value={option.value || emptyValue}
                >
                  {option.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      )}
    </ControlField>
  );
}

export function ReadinessMessage({
  label,
  message,
  tone,
}: Readonly<ReadinessMessageProps>) {
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
}: Readonly<{
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: Option[];
  counts?: Record<string, number>;
}>) {
  return (
    <fieldset className="segmented-filter">
      <legend className="sr-only">{label}</legend>
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
    </fieldset>
  );
}

export function Alert({
  children,
  tone,
}: ChildrenProps & { tone: Exclude<Tone, "neutral"> }) {
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

export function StatusBadge({ children, tone }: ToneChildrenProps) {
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

export function Kpi({ label, value }: Readonly<LabelValueProps>) {
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
}: Readonly<ProgressMeterProps>) {
  const ratio = max > 0 ? Math.min(Math.max(value / max, 0), 1) : 0;
  const percentage = Math.round(ratio * 100);
  return (
    <div
      className={compact ? "progress-meter compact" : "progress-meter"}
      aria-label={`${label} ${helper}`}
    >
      <div className="progress-meter-head">
        <span>{label}</span>
        <strong>{helper}</strong>
      </div>
      <progress
        className="progress-track"
        value={percentage}
        max={100}
        aria-label={label}
      >
        <span />
      </progress>
    </div>
  );
}

function slugify(value: string) {
  return value.split(/\s+/).filter(Boolean).join("-").toLowerCase();
}

export function EmptyState({ title, action }: Readonly<EmptyStateProps>) {
  return (
    <UiEmpty className="empty-state">
      <EmptyHeader>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{action}</EmptyDescription>
      </EmptyHeader>
    </UiEmpty>
  );
}

export function DangerZonePanel({ children }: Readonly<ChildrenProps>) {
  return <Card className="danger-zone-panel">{children}</Card>;
}

export function ResponsiveTable({
  children,
  label = "資料表",
  mobileCards,
}: ResponsiveTableProps) {
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
        aria-label={label}
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

export function SkeletonRows({ rows }: Readonly<{ rows: number }>) {
  return (
    <div className="skeleton-stack" role="status" aria-live="polite">
      <span className="sr-only">載入中…</span>
      {Array.from({ length: rows }, (_, index) => (
        <Skeleton className="skeleton-row" key={index} />
      ))}
    </div>
  );
}
