import { useEffect, useState } from "react";
import { Icon } from "@/components/shared/icon";
import type { EventSummary } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { EventPoster } from "./employee-event-poster";

const maxPosterBytes = 5 * 1024 * 1024;
const acceptedPosterTypes = ["image/jpeg", "image/png", "image/webp"];

export function AdminEventPosterField({
  busy,
  event,
  helper,
  posterFile,
  posterVersion = 0,
  status,
  title,
  onPosterSelect,
}: Readonly<{
  busy: boolean;
  event?: EventSummary;
  helper: string;
  posterFile: File | null;
  posterVersion?: number;
  status?: string;
  title: string;
  onPosterSelect: (file: File | null) => void;
}>) {
  const [localPreviewURL, setLocalPreviewURL] = useState("");
  const [validationMessage, setValidationMessage] = useState("");

  useEffect(() => {
    if (!posterFile) {
      setLocalPreviewURL("");
      return undefined;
    }
    const objectURL = URL.createObjectURL(posterFile);
    setLocalPreviewURL(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [posterFile]);

  function selectPoster(file: File | null) {
    if (!file) return;
    const nextValidationMessage = posterValidationMessage(file);
    setValidationMessage(nextValidationMessage);
    if (nextValidationMessage) {
      onPosterSelect(null);
      return;
    }
    onPosterSelect(file);
  }

  return (
    <fieldset className="form-section full admin-poster-field">
      <legend>活動海報</legend>
      <div className="admin-poster-preview">
        <div className="admin-poster-frame">
          {localPreviewURL ? (
            <img alt={`${title || "活動"} 海報預覽`} src={localPreviewURL} />
          ) : (
            <EventPoster
              eventID={event?.event_id || ""}
              key={`${event?.event_id || "draft"}-${posterVersion}`}
              meta={eventPosterMeta(event)}
              showFallbackTitle
              title={title || event?.title || "活動海報"}
              variant="hero"
            />
          )}
        </div>
        <div className="admin-poster-copy">
          <strong>{localPreviewURL ? "本機預覽" : "目前海報"}</strong>
          <span>{posterFile ? posterFileSummary(posterFile) : helper}</span>
          {(status || validationMessage) && (
            <small aria-live="polite">{validationMessage || status}</small>
          )}
        </div>
      </div>
      <label className="field admin-poster-upload">
        <span>活動海報</span>
        <input
          accept={acceptedPosterTypes.join(",")}
          disabled={busy}
          type="file"
          onChange={(event) => {
            selectPoster(event.currentTarget.files?.[0] ?? null);
            event.currentTarget.value = "";
          }}
        />
      </label>
      <p className="form-hint">
        JPG、PNG、WebP，最多 5MB。沒有海報時會使用預設彩色封面。
      </p>
      {posterFile && (
        <Button
          className="admin-poster-clear"
          type="button"
          variant="outline"
          onClick={() => {
            setValidationMessage("");
            onPosterSelect(null);
          }}
          disabled={busy}
        >
          <Icon name="x" />
          清除選取
        </Button>
      )}
    </fieldset>
  );
}

function eventPosterMeta(event?: EventSummary) {
  if (!event) return "預設彩色封面";
  return event.location || event.event_site || "活動海報";
}

function posterFileSummary(file: File) {
  const size = `${(file.size / 1024 / 1024).toFixed(2)} MB`;
  return `${file.name} · ${file.type || "未知格式"} · ${size}`;
}

function posterValidationMessage(file: File) {
  if (!acceptedPosterTypes.includes(file.type)) {
    return "海報格式需為 JPG、PNG 或 WebP。";
  }
  if (file.size > maxPosterBytes) {
    return "海報必須小於或等於 5MB。";
  }
  return "";
}
