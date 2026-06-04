import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getTicket, listTickets } from "@/lib/api";
import type { AuthMeClaims, Ticket } from "@/lib/api";
import { EmployeeTicketsPage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    getTicket: vi.fn(),
    listTickets: vi.fn(),
  };
});

const mockGetTicket = vi.mocked(getTicket);
const mockListTickets = vi.mocked(listTickets);

const claims: AuthMeClaims = {
  employee_id: "E1001",
  display_name: "Ariel Chen",
  role_claims: ["employee"],
  mapped_roles: ["employee"],
  department: "Engineering",
  site: "Taipei",
  city: "Taipei",
  claims_status: "complete",
};

describe("EmployeeTicketsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.replaceState({}, "", "/user/tickets");
  });

  it("renders the current ticket QR before the rest of the ticket list", async () => {
    mockListTickets.mockResolvedValue([
      ticketFixture(),
      ticketFixture({ ticket_id: "T-2", status: "active", signed_token: "" }),
      ticketFixture({ ticket_id: "T-3", status: "redeemed" }),
      ticketFixture({ ticket_id: "T-4", status: "revoked" }),
      ticketFixture({
        ticket_id: "T-5",
        event_starts_at: "2099-05-19T10:00:00Z",
      }),
    ]);

    render(<EmployeeTicketsPage claims={claims} />);

    expect(await screen.findByText("我的票券")).toBeInTheDocument();
    expect(screen.getByText("目前可入場票券")).toBeInTheDocument();
    expect(screen.getByLabelText("票券二維碼")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /T-1/ })).toHaveAttribute(
      "href",
      "/user/tickets?ticket_id=T-1",
    );
    const summary = screen.getByLabelText("票券摘要");
    expect(summary).toHaveTextContent("可入場1");
    expect(summary).toHaveTextContent("尚未開放1");
    expect(summary).toHaveTextContent("待產生 QR1");
    expect(summary).toHaveTextContent("已核銷1");
    expect(summary).toHaveTextContent("已撤銷1");
    expect(mockGetTicket).not.toHaveBeenCalled();
  });

  it("opens the clicked ticket detail instead of selecting the first row", async () => {
    mockListTickets.mockResolvedValue([
      ticketFixture({ ticket_id: "T-1", event_title: "第一張票" }),
      ticketFixture({ ticket_id: "T-2", event_title: "第二張票" }),
    ]);
    mockGetTicket.mockResolvedValue(
      ticketFixture({ ticket_id: "T-2", event_title: "第二張票" }),
    );

    render(<EmployeeTicketsPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("link", { name: /第二張票/ }),
    );

    expect(mockGetTicket).toHaveBeenCalledWith("T-2");
    expect(await screen.findByText("票券詳細")).toBeInTheDocument();
    expect(screen.getByText("第二張票")).toBeInTheDocument();
    expect(window.location.search).toBe("?ticket_id=T-2");
  });

  it("loads a direct ticket detail URL without listing tickets", async () => {
    window.history.replaceState({}, "", "/user/tickets?ticket_id=T-2");
    mockGetTicket.mockResolvedValue(
      ticketFixture({ ticket_id: "T-2", event_title: "直接開啟票券" }),
    );

    render(<EmployeeTicketsPage claims={claims} />);

    await waitFor(() => expect(mockGetTicket).toHaveBeenCalledWith("T-2"));
    expect(mockListTickets).not.toHaveBeenCalled();
    expect(await screen.findByText("直接開啟票券")).toBeInTheDocument();
  });

  it("shows a recoverable error for missing ticket detail without fallback", async () => {
    window.history.replaceState({}, "", "/user/tickets?ticket_id=missing");
    mockGetTicket.mockRejectedValue(new Error("ticket not found"));

    render(<EmployeeTicketsPage claims={claims} />);

    expect(await screen.findByText("找不到票券。")).toBeInTheDocument();
    expect(mockListTickets).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("票券二維碼")).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "返回我的票券" }),
    ).toBeInTheDocument();
  });

  it("shows a recoverable error for forbidden ticket detail without fallback", async () => {
    window.history.replaceState({}, "", "/user/tickets?ticket_id=forbidden");
    mockGetTicket.mockRejectedValue(new Error("forbidden"));

    render(<EmployeeTicketsPage claims={claims} />);

    expect(
      await screen.findByText(/權限|forbidden|目前角色/),
    ).toBeInTheDocument();
    expect(mockListTickets).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("票券二維碼")).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "返回我的票券" }),
    ).toBeInTheDocument();
  });

  it("rejects mismatched ticket detail responses without showing a QR", async () => {
    window.history.replaceState({}, "", "/user/tickets?ticket_id=T-expected");
    mockGetTicket.mockResolvedValue(
      ticketFixture({ ticket_id: "T-other", event_title: "錯誤票券" }),
    );

    render(<EmployeeTicketsPage claims={claims} />);

    expect(
      await screen.findByText("票券資料不一致，請返回清單後重新開啟。"),
    ).toBeInTheDocument();
    expect(screen.queryByText("錯誤票券")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("票券二維碼")).not.toBeInTheDocument();
  });

  it("keeps raw token values out of visible ticket detail text", async () => {
    window.history.replaceState({}, "", "/user/tickets?ticket_id=T-1");
    mockGetTicket.mockResolvedValue(
      ticketFixture({
        signed_token: "signed-secret",
        qr_payload: "qr-secret",
      }),
    );

    render(<EmployeeTicketsPage claims={claims} />);

    expect(await screen.findByLabelText("票券二維碼")).toBeInTheDocument();
    expect(screen.queryByText("signed-secret")).not.toBeInTheDocument();
    expect(screen.queryByText("qr-secret")).not.toBeInTheDocument();
  });

  it("shows revoked reason only in metadata, not duplicated in status copy", async () => {
    window.history.replaceState({}, "", "/user/tickets?ticket_id=T-revoked");
    mockGetTicket.mockResolvedValue(
      ticketFixture({
        ticket_id: "T-revoked",
        status: "revoked",
        revoked_reason: "員工已取消報名",
      }),
    );

    render(<EmployeeTicketsPage claims={claims} />);

    expect(await screen.findByText("票券詳細")).toBeInTheDocument();
    expect(screen.getByText("此票券已撤銷，不能入場。")).toBeInTheDocument();
    expect(screen.getByText("撤銷原因")).toBeInTheDocument();
    expect(screen.getByText("員工已取消報名")).toBeInTheDocument();
    expect(screen.queryByText(/原因：員工已取消報名/)).not.toBeInTheDocument();
  });
});

function ticketFixture(overrides: Partial<Ticket> = {}): Ticket {
  return {
    ticket_id: "T-1",
    registration_id: "R-1",
    event_id: "EVT-1",
    employee_id: "E1001",
    status: "active",
    signed_token: "signed-token",
    issued_at: "2026-05-06T10:00:00Z",
    expires_at: "2099-12-31T23:59:59Z",
    event_title: "台北家庭電影夜",
    event_location: "Taipei HQ",
    event_starts_at: "2026-06-04T00:00:00Z",
    employee_name: "Ariel Chen",
    non_transferable: true,
    ...overrides,
  };
}
