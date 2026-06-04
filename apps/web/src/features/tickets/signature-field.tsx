import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/shared/icon";

const MASK = "••••••••••••••••••••••••";

export function TicketSignatureField({ token }: Readonly<{ token: string }>) {
  const [revealed, setRevealed] = useState(false);
  const [copied, setCopied] = useState(false);

  if (!token) return null;

  async function copy() {
    try {
      await globalThis.navigator.clipboard.writeText(token);
      setCopied(true);
      globalThis.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className="ticket-signature">
      <div className="ticket-signature-head">
        <span className="ticket-signature-label">票券簽章碼</span>
        <span className="form-hint">QR 無法掃描時，驗票員可手動貼上此碼。</span>
      </div>
      <code className="ticket-signature-code" aria-label="票券簽章碼">
        {revealed ? token : MASK}
      </code>
      <div className="ticket-signature-actions">
        <Button
          variant="outline"
          size="sm"
          type="button"
          aria-pressed={revealed}
          onClick={() => setRevealed((value) => !value)}
        >
          <Icon name={revealed ? "eyeOff" : "eye"} />
          {revealed ? "隱藏簽章碼" : "顯示簽章碼"}
        </Button>
        <Button
          variant="outline"
          size="sm"
          type="button"
          onClick={() => void copy()}
        >
          <Icon name="copy" />
          {copied ? "已複製" : "複製簽章碼"}
        </Button>
      </div>
    </div>
  );
}
