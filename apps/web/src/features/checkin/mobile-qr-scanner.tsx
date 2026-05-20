import { useEffect, useRef, useState } from "react";
import { Alert } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";

type BarcodeResult = {
  rawValue?: string;
};

type BarcodeDetectorInstance = {
  detect(source: HTMLVideoElement): Promise<BarcodeResult[]>;
};

type BarcodeDetectorConstructor = new (options?: {
  formats?: string[];
}) => BarcodeDetectorInstance;

type ScannerState = "idle" | "starting" | "scanning" | "done" | "unavailable";

const CAMERA_UNAVAILABLE_COPY =
  "此瀏覽器不支援直接相機辨識 QR code，請使用手機內建掃描器或手動貼上票券簽章碼。";

export function MobileQrScanner({
  onTokenDetected,
}: {
  onTokenDetected: (token: string) => void;
}) {
  const [state, setState] = useState<ScannerState>("idle");
  const [message, setMessage] = useState("");
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const frameRef = useRef(0);

  useEffect(() => () => stopCamera(false), []);

  async function startScanner() {
    setMessage("");
    const Detector = barcodeDetector();
    if (!Detector || !navigator.mediaDevices?.getUserMedia) {
      setState("unavailable");
      setMessage(CAMERA_UNAVAILABLE_COPY);
      return;
    }

    setState("starting");
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: { ideal: "environment" },
        },
        audio: false,
      });
      streamRef.current = stream;
      if (!videoRef.current) return;
      videoRef.current.srcObject = stream;
      await videoRef.current.play();
      setState("scanning");
      scanFrame(new Detector({ formats: ["qr_code"] }));
    } catch {
      stopCamera();
      setState("unavailable");
      setMessage(
        "無法啟動相機。請允許瀏覽器相機權限，或改用手動貼上票券簽章碼。",
      );
    }
  }

  function scanFrame(detector: BarcodeDetectorInstance) {
    const video = videoRef.current;
    if (!video) return;

    void detector
      .detect(video)
      .then((codes) => {
        const token = codes.find((code) => code.rawValue)?.rawValue?.trim();
        if (token) {
          onTokenDetected(token);
          setState("done");
          setMessage("已讀取 QR code，可以送出驗票。");
          stopCamera(false);
          return;
        }
        frameRef.current = window.requestAnimationFrame(() =>
          scanFrame(detector),
        );
      })
      .catch(() => {
        frameRef.current = window.requestAnimationFrame(() =>
          scanFrame(detector),
        );
      });
  }

  function stopCamera(resetState = true) {
    if (frameRef.current) window.cancelAnimationFrame(frameRef.current);
    frameRef.current = 0;
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
    if (videoRef.current) videoRef.current.srcObject = null;
    if (resetState) {
      setState((current) => (current === "scanning" ? "idle" : current));
    }
  }

  const scanning = state === "starting" || state === "scanning";

  return (
    <div className="mobile-qr-scanner" aria-live="polite">
      <div className="mobile-qr-scanner-actions">
        <Button
          type="button"
          onClick={() => void startScanner()}
          disabled={scanning}
        >
          <Icon name="scan" />
          {state === "starting"
            ? "啟動相機"
            : state === "scanning"
              ? "掃描中"
              : "手機掃描 QR"}
        </Button>
        <Button
          variant="outline"
          type="button"
          onClick={() => stopCamera()}
          disabled={!scanning}
        >
          停止
        </Button>
      </div>
      <div className="mobile-qr-preview" data-state={state}>
        <video
          ref={videoRef}
          muted
          playsInline
          aria-label="QR code 相機預覽"
        />
        {state !== "scanning" && state !== "starting" && (
          <span>相機預覽會在開始掃描後顯示</span>
        )}
      </div>
      {message && (
        <Alert tone={state === "unavailable" ? "warn" : "ok"}>{message}</Alert>
      )}
    </div>
  );
}

function barcodeDetector() {
  return (
    window as typeof window & {
      BarcodeDetector?: BarcodeDetectorConstructor;
    }
  ).BarcodeDetector;
}
