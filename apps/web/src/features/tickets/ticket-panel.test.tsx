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
    };

    render(<TicketPanel ticket={ticket} />);

    expect(screen.getByText("台北家庭電影夜")).toBeInTheDocument();
    expect(screen.getByText("T-1")).toBeInTheDocument();
    expect(screen.getByLabelText(/QR Code/i)).toBeInTheDocument();
    expect(screen.queryByText("signed-secret")).not.toBeInTheDocument();
  });

  it("supports compact rendering for narrow event detail panels", () => {
    const ticket: Ticket = {
      ticket_id: "tic_1234567890abcdef1234567890abcdef",
      registration_id: "R-1",
      event_id: "evt_1234567890abcdef1234567890abcdef",
      employee_id: "E1001",
      status: "active",
      signed_token: "signed-secret",
      issued_at: "2026-05-06T10:00:00Z",
    };

    const { container } = render(<TicketPanel compact ticket={ticket} />);

    expect(
      container.querySelector(".ticket-panel.compact"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("evt_1234567890abcdef1234567890abcdef"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("tic_1234567890abcdef1234567890abcdef"),
    ).toBeInTheDocument();
  });

  it("uses in-memory checkin handoff instead of localStorage", async () => {
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
    };

    const pushStateSpy = vi.spyOn(window.history, "pushState");
    const setItemSpy = vi.spyOn(Storage.prototype, "setItem");
    render(<TicketPanel ticket={ticket} />);

    await userEvent.click(screen.getByRole("button", { name: "帶到驗票頁" }));

    expect(pushStateSpy).toHaveBeenCalledWith(
      { cetsCheckinToken: "signed-secret" },
      "",
      "/admin/checkin",
    );
    expect(setItemSpy).not.toHaveBeenCalled();
    pushStateSpy.mockRestore();
    setItemSpy.mockRestore();
  });
});
