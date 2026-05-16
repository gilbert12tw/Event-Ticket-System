import { useMemo, useState } from "react";
import {
  EmptyState,
  Field,
  SelectField,
  StatusBadge,
  TextareaField,
} from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Field as UiField, FieldLabel } from "@/components/ui/field";
import { splitTags } from "@/lib/formatting";
import {
  capacityTypeOptions,
  eligibilityDepartmentOptions,
  eligibilitySiteOptions,
  employmentStatusOptions,
  gradeOptions,
  tagSuggestionOptions,
  venueOptions,
} from "@/lib/ui/options";
import type { AdminCreateForm, AdminEditForm } from "./admin-event-crud-types";
import { Button } from "@/components/ui/button";

export function EventCoreFields<T extends AdminCreateForm | AdminEditForm>({
  capacityReady,
  form,
  onChange,
  windowReady,
}: {
  capacityReady: boolean;
  form: T;
  onChange: (next: T) => void;
  windowReady: boolean;
}) {
  return (
    <>
      <fieldset className="form-section full">
        <legend>基本資料</legend>
        <Field
          label="活動名稱"
          name="event-title"
          value={form.title}
          onChange={(title) => onChange({ ...form, title })}
          required
        />
        <SelectField
          label="地點"
          name="event-location"
          value={form.location}
          options={venueOptions}
          onChange={(location) => onChange({ ...form, location })}
        />
        <TextareaField
          className="full"
          label="描述"
          name="event-description"
          value={form.description}
          onChange={(description) => onChange({ ...form, description })}
          rows={4}
        />
      </fieldset>
      <fieldset className="form-section full">
        <legend>時間與容量</legend>
        <Field
          label="活動開始"
          name="event-starts-at"
          type="datetime-local"
          value={form.starts_at}
          onChange={(starts_at) => onChange({ ...form, starts_at })}
          hint="活動開始需晚於報名截止。"
          invalid={!windowReady}
          required
        />
        <Field
          label="報名開始"
          name="registration-start"
          type="datetime-local"
          value={form.registration_start}
          onChange={(registration_start) =>
            onChange({ ...form, registration_start })
          }
          hint="報名開始不可晚於報名截止。"
          invalid={!windowReady}
          required
        />
        <Field
          label="報名截止"
          name="registration-close"
          type="datetime-local"
          value={form.registration_close}
          onChange={(registration_close) =>
            onChange({ ...form, registration_close })
          }
          hint="建議至少早於活動開始 24 小時。"
          invalid={!windowReady}
          required
        />
        <SelectField
          label="票數類型"
          value={form.capacity_type}
          options={capacityTypeOptions}
          onChange={(value) => {
            const capacity_type = value as "limited" | "unlimited";
            onChange({
              ...form,
              capacity_type,
              capacity:
                capacity_type === "unlimited" ? "" : form.capacity || "1",
            });
          }}
        />
        {form.capacity_type === "limited" ? (
          <Field
            label="容量"
            name="event-capacity"
            type="number"
            inputMode="numeric"
            value={form.capacity}
            onChange={(capacity) => onChange({ ...form, capacity })}
            hint="限量活動必須大於 0。"
            invalid={!capacityReady}
            required
          />
        ) : (
          <p className="form-hint full">
            不限量活動不設總名額，員工報名時可填寫攜帶家屬人數。
          </p>
        )}
      </fieldset>
    </>
  );
}

export function EligibilityFields({
  form,
  onChange,
}: {
  form: AdminCreateForm;
  onChange: (next: AdminCreateForm) => void;
}) {
  return (
    <fieldset className="form-section full">
      <legend>資格規則</legend>
      <SelectField
        label="部門"
        value={form.department || "*"}
        options={eligibilityDepartmentOptions}
        onChange={(department) => onChange({ ...form, department })}
      />
      <SelectField
        label="廠區"
        value={form.site || "*"}
        options={eligibilitySiteOptions}
        onChange={(site) => onChange({ ...form, site })}
      />
      <SelectField
        label="最低職等"
        value={form.min_grade}
        options={gradeOptions}
        onChange={(min_grade) => onChange({ ...form, min_grade })}
      />
      <SelectField
        label="雇用狀態"
        value={form.employment_status || "*"}
        options={employmentStatusOptions}
        onChange={(employment_status) =>
          onChange({ ...form, employment_status })
        }
      />
    </fieldset>
  );
}

export function IntentSelector({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="status-selectors" role="radiogroup" aria-label="建立意圖">
      {[
        { value: "draft", label: "儲存草稿" },
        { value: "published", label: "檢查通過後發布" },
      ].map((option) => (
        <label className="intent-option" key={option.value}>
          <input
            type="radio"
            name="create-intent"
            checked={value === option.value}
            onChange={() => onChange(option.value)}
          />
          <span>{option.label}</span>
        </label>
      ))}
    </div>
  );
}

export function TagInput({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const [custom, setCustom] = useState("");
  const tags = useMemo(() => splitTags(value), [value]);
  const options = tagSuggestionOptions.filter(
    (option) => !tags.includes(option.value),
  );

  function addTag(tag: string) {
    if (!tag.trim()) return;
    onChange([...tags, tag.trim()].join(", "));
    setCustom("");
  }

  return (
    <UiField className="tag-field full">
      <FieldLabel>標籤</FieldLabel>
      <div className="tag-input">
        <SelectField
          label="新增標籤"
          value=""
          options={[{ value: "", label: "選擇建議標籤" }, ...options]}
          onChange={addTag}
        />
        <div className="tag-custom">
          <Field
            label="自訂標籤"
            name="custom-tag"
            value={custom}
            onChange={setCustom}
          />
          <Button
            variant="outline"
            type="button"
            onClick={() => addTag(custom)}
            disabled={!custom.trim()}
          >
            新增
          </Button>
        </div>
        <div className="tag-chip-row" aria-label="已選標籤">
          {tags.length === 0 && (
            <span className="form-hint">尚未加入標籤。</span>
          )}
          {tags.map((tag) => (
            <Button
              aria-label={`移除標籤 ${tag}`}
              className="tag-chip"
              key={tag}
              size="xs"
              type="button"
              variant="outline"
              onClick={() =>
                onChange(
                  tags.filter((candidate) => candidate !== tag).join(", "),
                )
              }
            >
              {tag}
              <Icon name="x" />
            </Button>
          ))}
        </div>
      </div>
    </UiField>
  );
}

export function ReadinessGrid({
  checks,
}: {
  checks: { label: string; ok: boolean }[];
}) {
  return (
    <div className="readiness-grid full" aria-label="發布檢查">
      {checks.map((item) => (
        <div
          className={item.ok ? "readiness-item ok" : "readiness-item warn"}
          key={item.label}
        >
          <StatusBadge tone={item.ok ? "ok" : "warn"}>
            {item.ok ? "通過" : "需確認"}
          </StatusBadge>
          <span>{item.label}</span>
        </div>
      ))}
    </div>
  );
}

export function SelectEventFirst({ action }: { action: string }) {
  return (
    <EmptyState
      title="尚未選擇活動"
      action={`請先回到活動清單選取活動，再${action}。`}
    />
  );
}
