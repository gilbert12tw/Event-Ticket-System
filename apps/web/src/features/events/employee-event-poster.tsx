import { useEffect, useState } from "react";
import { eventPosterBlob } from "@/lib/api";

export function EventPoster({
  eventID,
  title,
  variant = "card",
}: Readonly<{
  eventID: string;
  title: string;
  variant?: "card" | "hero";
}>) {
  const posterURL = useEventPoster(eventID);
  const className =
    variant === "hero" ? "employee-event-poster hero" : "employee-event-poster";
  return (
    <div className={className}>
      {posterURL ? (
        <img alt={`${title} 海報`} src={posterURL} />
      ) : (
        <div className="employee-event-poster-fallback" aria-hidden="true">
          <span>{posterInitial(title)}</span>
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
