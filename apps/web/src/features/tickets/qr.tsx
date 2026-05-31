import { QRCodeSVG } from "qrcode.react";

const QR_SIZE = 180;

export function TicketQrCode({ token }: Readonly<{ token: string }>) {
  if (!token) {
    return (
      <div className="qr qr-empty" aria-label="二維碼無法使用">
        <span className="form-hint">尚無可產生之票券資料</span>
      </div>
    );
  }

  return (
    <div className="qr" aria-label="票券二維碼">
      <QRCodeSVG
        value={token}
        size={QR_SIZE}
        level="M"
        marginSize={4}
        title="票券二維碼"
      />
    </div>
  );
}
