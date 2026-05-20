import { useEffect, useRef, useState } from "react";
import { BrowserQRCodeReader } from "@zxing/browser";
import type { IScannerControls } from "@zxing/browser";
import { Alert } from "@/components/shared";
import { Icon } from "@/components/shared/icon";
import { Button } from "@/components/ui/button";

type ScannerState = "idle" | "starting" | "scanning" | "done" | "unavailable";

const CAMERA_UNAVAILABLE_COPY =
  "此瀏覽器無法開啟相機。請確認使用 HTTPS、允許 Safari 相機權限，或改用手動貼上票券簽章碼。";

export function MobileQrScanner({
  onTokenDetected,
}: {
  onTokenDetected: (token: string) => void;
}) {
  const [state, setState] = useState<ScannerState>("idle");
  const [message, setMessage] = useState("");
  const controlsRef = useRef<IScannerControls | null>(null);
  const mountedRef = useRef(false);
  const scannerActiveRef = useRef(false);
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      stopCamera(false);
    };
  }, []);

  async function startScanner() {
    setMessage("");
    if (!navigator.mediaDevices?.getUserMedia || !videoRef.current) {
      setState("unavailable");
      setMessage(CAMERA_UNAVAILABLE_COPY);
      return;
    }

    scannerActiveRef.current = true;
    setState("starting");
    try {
      const reader = new BrowserQRCodeReader();
      const controls = await reader.decodeFromConstraints(
        {
          video: {
            facingMode: { ideal: "environment" },
          },
          audio: false,
        },
        videoRef.current,
        (result) => {
          if (!scannerActiveRef.current || !result) return;
          const token = result.getText().trim();
          if (!token) return;
          onTokenDetected(token);
          setState("done");
          setMessage("已讀取 QR code，可以送出驗票。");
          stopCamera(false);
        },
      );
      if (!scannerActiveRef.current) {
        controls.stop();
        return;
      }
      controlsRef.current = controls;
      setState("scanning");
      setMessage("相機已開啟，請將 QR code 對準畫面中央。");
    } catch {
      const wasActive = scannerActiveRef.current;
      stopCamera();
      if (mountedRef.current && wasActive) {
        setState("unavailable");
        setMessage(
          "無法啟動相機。請允許瀏覽器相機權限，或改用手動貼上票券簽章碼。",
        );
      }
    }
  }

  function stopCamera(resetState = true) {
    scannerActiveRef.current = false;
    controlsRef.current?.stop();
    controlsRef.current = null;
    if (videoRef.current) videoRef.current.srcObject = null;
    if (resetState && mountedRef.current) {
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
            ? "啟動中..."
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
        <video ref={videoRef} muted playsInline aria-label="QR code 相機預覽" />
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
