import { QRCodeSVG } from "qrcode.react";

const QR_SIZE = 180;

export function TicketQrCode({ token }: { token: string }) {
  if (!token) {
    return (
      <div className="qr qr-empty" aria-label="QR Code unavailable">
        <span className="form-hint">尚無可產生之票券資料</span>
      </div>
    );
  }

  return (
    <div className="qr" aria-label="QR Code visual">
      <QRCodeSVG value={token} size={QR_SIZE} level="M" marginSize={4} title="Ticket QR Code" />
    </div>
  );
}
