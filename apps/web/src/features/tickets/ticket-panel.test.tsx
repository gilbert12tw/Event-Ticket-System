import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TicketPanel } from "./pages";
import type { Ticket } from "@/lib/api";

describe("TicketPanel", () => {
  it("renders an empty state when no ticket exists", () => {
    render(<TicketPanel />);

    expect(screen.getByText("沒有可顯示的票券")).toBeInTheDocument();
  });

  it("renders ticket metadata while keeping token details out of ordinary text", () => {
    const ticket: Ticket = {
      ticket_id: "T-1",
      registration_id: "R-1",
      event_id: "EVT-1",
      employee_id: "E1001",
      status: "active",
      signed_token: "signed-secret",
      qr_payload: "qr-secret",
      issued_at: "2026-05-06T10:00:00Z",
      event_title: "台北家庭電影夜",
      event_location: "Taipei HQ",
      employee_name: "Ariel Chen",
      department: "Engineering",
      city: "Taipei",
      family_count: 2,
      non_transferable: true,
    };

    const { container } = render(<TicketPanel ticket={ticket} />);

    expect(screen.getByText("台北家庭電影夜")).toBeInTheDocument();
    expect(screen.getByText("不可轉讓")).toBeInTheDocument();
    expect(screen.getByText("入場提示")).toBeInTheDocument();
    expect(
      screen.getByText(
        "請在入口出示此 QR code，驗票員完成核銷後票券會更新為已核銷。",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("同行人數")).toBeInTheDocument();
    expect(
      screen.getByText("僅供入場人數核對，非可轉讓票券。"),
    ).toBeInTheDocument();
    expect(screen.getByText("Engineering")).toBeInTheDocument();
    expect(screen.getByText("T-1")).toBeInTheDocument();
    expect(screen.getByLabelText("票券二維碼")).toBeInTheDocument();
    expect(
      container.querySelector(".ticket-detail .qr-wrap"),
    ).toBeInTheDocument();
    expect(screen.queryByText("signed-secret")).not.toBeInTheDocument();
  });

  it("keeps compact ticket panels focused on entry details", () => {
    const ticket: Ticket = {
      ticket_id: "tic_1234567890abcdef1234567890abcdef",
      registration_id: "R-1",
      event_id: "evt_1234567890abcdef1234567890abcdef",
      employee_id: "E1001",
      event_location: "Taipei HQ",
      event_starts_at: "2026-05-06T10:00:00Z",
      status: "active",
      signed_token: "signed-secret",
      issued_at: "2026-05-06T10:00:00Z",
      non_transferable: true,
    };

    const { container } = render(<TicketPanel compact ticket={ticket} />);

    expect(
      container.querySelector(".ticket-panel.compact"),
    ).toBeInTheDocument();
    expect(screen.getByText("活動票券")).toBeInTheDocument();
    expect(screen.getByText("地點")).toBeInTheDocument();
    expect(container).not.toHaveTextContent(
      "evt_1234567890abcdef1234567890abcdef",
    );
    expect(container).not.toHaveTextContent(
      "tic_1234567890abcdef1234567890abcdef",
    );
    expect(container).not.toHaveTextContent("E1001");
  });

  it("renders unavailable tickets as a single non-QR state without repeated copy", () => {
    const ticket: Ticket = {
      ticket_id: "T-redeemed",
      registration_id: "R-1",
      event_id: "EVT-1",
      employee_id: "E1001",
      status: "redeemed",
      signed_token: "signed-secret",
      issued_at: "2026-05-06T10:00:00Z",
      event_title: "台北家庭電影夜",
      non_transferable: true,
    };

    const { container } = render(<TicketPanel ticket={ticket} />);

    expect(
      container.querySelector(".ticket-panel.unavailable"),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("票券二維碼")).not.toBeInTheDocument();
    expect(screen.getAllByText("此票券已核銷，不能再次入場。")).toHaveLength(1);
    expect(screen.queryByText("signed-secret")).not.toBeInTheDocument();
  });

  it("renders active tickets without QR as a single pending state", () => {
    const onRefresh = vi.fn();
    const ticket: Ticket = {
      ticket_id: "T-pending",
      registration_id: "R-1",
      event_id: "EVT-1",
      employee_id: "E1001",
      status: "active",
      issued_at: "2026-05-06T10:00:00Z",
      event_title: "台北家庭電影夜",
      non_transferable: true,
    };

    const { container } = render(
      <TicketPanel ticket={ticket} onRefresh={onRefresh} />,
    );

    expect(
      container.querySelector(".ticket-panel.unavailable"),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("票券二維碼")).not.toBeInTheDocument();
    expect(
      screen.getAllByText(
        "二維碼尚未產生，請重新整理票券；若仍未出現，請聯絡活動主辦。",
      ),
    ).toHaveLength(1);
    expect(
      screen.getByRole("button", { name: "重新整理票券" }),
    ).toBeInTheDocument();
  });

  it("shows issued QR before entry opens without marking future tickets as entry-ready", () => {
    const ticket: Ticket = {
      ticket_id: "T-future",
      registration_id: "R-1",
      event_id: "EVT-1",
      employee_id: "E1001",
      status: "active",
      signed_token: "signed-secret",
      qr_payload: "qr-secret",
      issued_at: "2026-05-06T10:00:00Z",
      event_starts_at: "2099-05-06T10:00:00Z",
      event_title: "台北家庭電影夜",
    };

    render(<TicketPanel ticket={ticket} />);

    expect(screen.getByText("尚未開放入場")).toBeInTheDocument();
    expect(
      screen.getByText("活動尚未開始，請於開始時間到場後再出示二維碼驗票。"),
    ).toBeInTheDocument();
    expect(screen.queryByText("可入場")).not.toBeInTheDocument();
    expect(screen.getByLabelText("票券二維碼")).toBeInTheDocument();
    expect(screen.getByText("入場提示")).toBeInTheDocument();
  });

  it("keeps employees in their own workspace when opening event detail", async () => {
    const ticket: Ticket = {
      ticket_id: "T-2",
      registration_id: "R-2",
      event_id: "EVT-2",
      employee_id: "E1001",
      status: "active",
      signed_token: "signed-secret",
      issued_at: "2026-05-06T10:00:00Z",
      event_title: "台北家庭電影夜",
      event_location: "Taipei HQ",
      employee_name: "Ariel Chen",
      non_transferable: true,
    };

    const pushStateSpy = vi.spyOn(globalThis.history, "pushState");
    const setItemSpy = vi.spyOn(Storage.prototype, "setItem");
    render(<TicketPanel ticket={ticket} />);

    await userEvent.click(screen.getByRole("link", { name: "查看活動詳情" }));

    expect(pushStateSpy).toHaveBeenCalledWith(
      {},
      "",
      "/user/events/detail?event_id=EVT-2",
    );
    expect(setItemSpy).not.toHaveBeenCalled();
    pushStateSpy.mockRestore();
    setItemSpy.mockRestore();
  });
});
