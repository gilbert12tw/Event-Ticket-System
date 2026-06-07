import { useEffect, useState } from "react";
import { eventPosterBlob } from "@/lib/api";

export function EventPoster({
  eventID,
  meta,
  showFallbackTitle = false,
  title,
  variant = "card",
}: Readonly<{
  eventID: string;
  meta?: string;
  showFallbackTitle?: boolean;
  title: string;
  variant?: "card" | "hero";
}>) {
  const posterURL = useEventPoster(eventID);
  const [imageRejected, setImageRejected] = useState(false);
  useEffect(() => setImageRejected(false), [posterURL]);
  const className = [
    "employee-event-poster",
    variant === "hero" ? "hero" : "",
    `cover-${posterCoverTone(eventID, title)}`,
  ]
    .filter(Boolean)
    .join(" ");
  const dimensions =
    variant === "hero"
      ? { width: 1200, height: 675 }
      : { width: 960, height: 600 };
  const showPosterImage = Boolean(posterURL) && !imageRejected;
  return (
    <div className={className}>
      {showPosterImage ? (
        <img
          alt={`${title} 海報`}
          decoding="async"
          fetchPriority={variant === "hero" ? "high" : "auto"}
          height={dimensions.height}
          loading={variant === "hero" ? "eager" : "lazy"}
          sizes={
            variant === "hero"
              ? "(max-width: 900px) 100vw, 66vw"
              : "(max-width: 760px) 100vw, 360px"
          }
          src={posterURL}
          width={dimensions.width}
          onError={() => setImageRejected(true)}
          onLoad={(event) => {
            const image = event.currentTarget;
            if (image.naturalWidth <= 2 && image.naturalHeight <= 2) {
              setImageRejected(true);
            }
          }}
        />
      ) : (
        <div className="employee-event-poster-fallback" aria-hidden="true">
          <span>{posterInitial(title)}</span>
          {showFallbackTitle && <strong>{title}</strong>}
          <small>{meta || "企業活動"}</small>
        </div>
      )}
    </div>
  );
}

export function useEventPoster(eventID: string) {
  const [posterURL, setPosterURL] = useState("");

  useEffect(() => {
    let active = true;
    let objectURL = "";
    if (!eventID) {
      setPosterURL("");
      return undefined;
    }
    void eventPosterBlob(eventID)
      .then((blob) => {
        if (!active || !blob) return;
        objectURL = URL.createObjectURL(blob);
        setPosterURL(objectURL);
      })
      .catch(() => {
        if (active) setPosterURL("");
      });
    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [eventID]);

  return posterURL;
}

function posterInitial(title: string) {
  const trimmed = title.trim();
  return trimmed ? trimmed.slice(0, 1).toUpperCase() : "C";
}

function posterCoverTone(eventID: string, title: string) {
  const seed = `${eventID}:${title}`;
  const total = Array.from(seed).reduce(
    (sum, char) => sum + (char.codePointAt(0) ?? 0),
    0,
  );
  return ["accent", "info", "success", "warn"][total % 4];
}
