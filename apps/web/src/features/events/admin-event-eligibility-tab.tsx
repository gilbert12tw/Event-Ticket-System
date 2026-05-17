import { useEffect, useState } from "react";
import {
  Alert,
  EmptyState,
  SelectField,
  StatusBadge,
} from "@/components/shared";
import { Button } from "@/components/ui/button";
import { previewEligibility, updateEligibility } from "@/lib/api";
import type {
  EligibilityPreviewResponse,
  EligibilityRuleInput,
  EventSummary,
} from "@/lib/api";
import { errorMessage } from "@/lib/formatting";
import {
  eligibilityDepartmentOptions,
  eligibilitySiteOptions,
  employmentStatusOptions,
  gradeOptions,
} from "@/lib/ui/options";
import { SelectEventFirst } from "./admin-event-form-controls";

export function AdminEventEligibilityTab({
  selectedEvent,
  onSaved,
}: {
  selectedEvent?: EventSummary;
  onSaved: (eventID: string) => void;
}) {
  const [form, setForm] = useState<EligibilityRuleInput>(() =>
    ruleFormFromEvent(selectedEvent),
  );
  const [preview, setPreview] = useState<EligibilityPreviewResponse | null>(
    null,
  );
  const [allowZeroMatch, setAllowZeroMatch] = useState(false);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setForm(ruleFormFromEvent(selectedEvent));
    setPreview(null);
    setAllowZeroMatch(false);
    setMessage("");
  }, [selectedEvent?.event_id]);

  if (!selectedEvent) return <SelectEventFirst action="檢視資格" />;

  async function previewImpact() {
    if (!selectedEvent) return;
    setBusy(true);
    setMessage("");
    try {
      const result = await previewEligibility(selectedEvent.event_id, {
        rule: form,
      });
      setPreview(result);
      setAllowZeroMatch(false);
      setMessage(
        result.zero_match
          ? "預覽完成：目前規則沒有命中任何員工。"
          : `預覽完成：${result.match_count} 人符合資格。`,
      );
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  async function saveRuleVersion() {
    if (!selectedEvent || (preview?.zero_match && !allowZeroMatch)) return;
    setBusy(true);
    setMessage("");
    try {
      const result = await updateEligibility(selectedEvent.event_id, {
        rule: form,
        allow_zero_match: Boolean(preview?.zero_match && allowZeroMatch),
      });
      setPreview(result);
      setMessage(`資格規則已儲存，符合 ${result.match_count} 人。`);
      onSaved(selectedEvent.event_id);
    } catch (error) {
      setMessage(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  const matchCount = preview?.match_count;
  const zeroMatchBlocked = Boolean(preview?.zero_match && !allowZeroMatch);

  return (
    <div className="task-panel">
      <div className="section-heading">
        <div>
          <h2>資格治理</h2>
          <p>先由後端預覽影響人數，再儲存新的資格規則版本。</p>
        </div>
        <StatusBadge tone={matchCount === 0 ? "warn" : "ok"}>
          {matchCount === undefined ? "尚未預覽" : `${matchCount} 人符合`}
        </StatusBadge>
      </div>
      <div className="form-grid">
        <SelectField
          label="部門"
          value={form.department || "*"}
          options={eligibilityDepartmentOptions}
          onChange={(department) => {
            setForm({ ...form, department });
            setPreview(null);
          }}
        />
        <SelectField
          label="廠區"
          value={form.site || "*"}
          options={eligibilitySiteOptions}
          onChange={(site) => {
            setForm({ ...form, site });
            setPreview(null);
          }}
        />
        <SelectField
          label="最低職等"
          value={String(form.min_grade)}
          options={gradeOptions}
          onChange={(min_grade) => {
            setForm({ ...form, min_grade: Number(min_grade) });
            setPreview(null);
          }}
        />
        <SelectField
          label="雇用狀態"
          value={form.employment_status || "*"}
          options={employmentStatusOptions}
          onChange={(employment_status) => {
            setForm({ ...form, employment_status });
            setPreview(null);
          }}
        />
        <div className="toolbar full">
          <Button
            variant="outline"
            type="button"
            onClick={() => void previewImpact()}
            disabled={busy}
          >
            Preview Impact
          </Button>
          <Button
            type="button"
            onClick={() => void saveRuleVersion()}
            disabled={busy || !preview || zeroMatchBlocked}
          >
            Save Rule Version
          </Button>
        </div>
      </div>
      {message && (
        <Alert tone={message.includes("已儲存") ? "ok" : "info"}>
          {message}
        </Alert>
      )}
      {preview?.zero_match && (
        <Alert tone="warn">
          目前資格規則命中 0 人。若這是有意安排，請勾選確認後再儲存。
        </Alert>
      )}
      {preview?.zero_match && (
        <label className="intent-option">
          <input
            type="checkbox"
            checked={allowZeroMatch}
            onChange={(event) => setAllowZeroMatch(event.currentTarget.checked)}
          />
          <span>我確認此規則可以儲存為 0 人命中版本</span>
        </label>
      )}
      {!preview && (
        <EmptyState
          title="尚未預覽資格影響"
          action="調整規則後先執行 Preview Impact，確認命中人數再儲存。"
        />
      )}
    </div>
  );
}

function ruleFormFromEvent(event?: EventSummary): EligibilityRuleInput {
  return {
    department: event?.rule.department || "*",
    site: event?.rule.site || "*",
    min_grade: event?.rule.min_grade ?? 1,
    employment_status: event?.rule.employment_status || "active",
  };
}
