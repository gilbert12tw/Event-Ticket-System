import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { eventPosterBlob, getTicket, listTickets } from "@/lib/api";
import type { AuthMeClaims, Ticket } from "@/lib/api";
import { EmployeeTicketsPage } from "./pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    eventPosterBlob: vi.fn(),
    getTicket: vi.fn(),
    listTickets: vi.fn(),
  };
});

const mockGetTicket = vi.mocked(getTicket);
const mockListTickets = vi.mocked(listTickets);
const mockEventPosterBlob = vi.mocked(eventPosterBlob);

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
    vi.unstubAllGlobals();
    mockEventPosterBlob.mockResolvedValue(null);
    window.history.replaceState({}, "", "/user/tickets");
  });

  it("renders the current ticket pass before the timeline without raw IDs", async () => {
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

    expect(
      await screen.findByRole("heading", { name: "我的票券清單" }),
    ).toHaveClass("sr-only");
    expect(screen.getByLabelText("目前可入場票券")).toBeInTheDocument();
    expect(screen.getByLabelText("票券二維碼")).toBeInTheDocument();
    expect(
      screen.getAllByRole("link", { name: /台北家庭電影夜/ })[0],
    ).toHaveAttribute("href", "/user/tickets?ticket_id=T-1");
    expect(screen.queryByLabelText("票券摘要")).not.toBeInTheDocument();
    expect(screen.queryByText("不可轉讓")).not.toBeInTheDocument();
    expect(screen.queryByText("入場提示")).not.toBeInTheDocument();
    expect(screen.queryByText("票券編號")).not.toBeInTheDocument();
    expect(screen.queryByText("T-1")).not.toBeInTheDocument();
    expect(screen.queryByText("E1001")).not.toBeInTheDocument();
    expect(screen.queryByText("signed-token")).not.toBeInTheDocument();
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

  it("keeps ticket usage guidance collapsed until the employee asks for it", async () => {
    window.history.replaceState({}, "", "/user/tickets?ticket_id=T-1");
    mockGetTicket.mockResolvedValue(ticketFixture());

    render(<EmployeeTicketsPage claims={claims} />);

    const summary = await screen.findByText("票券使用說明");
    const disclosure = summary.closest("details");
    expect(disclosure).not.toHaveAttribute("open");

    await userEvent.click(summary);

    expect(disclosure).toHaveAttribute("open");
  });

  it("keeps revoked tickets concise without employee-only metadata", async () => {
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
    expect(screen.queryByText("撤銷原因")).not.toBeInTheDocument();
    expect(screen.queryByText("員工已取消報名")).not.toBeInTheDocument();
    expect(screen.queryByText(/原因：員工已取消報名/)).not.toBeInTheDocument();
  });

  it("hides zero companion counts and exposes calendar export for active tickets", async () => {
    const createObjectURL = vi.fn(() => "blob:ticket-calendar");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", {
      createObjectURL,
      revokeObjectURL,
    });
    mockListTickets.mockResolvedValue([
      ticketFixture({ family_count: 0 }),
      ticketFixture({
        event_title: "雙人工作坊",
        family_count: 2,
        ticket_id: "T-family",
      }),
    ]);

    render(<EmployeeTicketsPage claims={claims} />);

    expect(await screen.findByText("雙人工作坊")).toBeInTheDocument();
    expect(screen.queryByText("同行 0 人")).not.toBeInTheDocument();
    expect(screen.getByText("同行 2 人")).toBeInTheDocument();

    await userEvent.click(
      screen.getAllByRole("button", { name: /加入行事曆/ })[0],
    );

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    const blob = createObjectURL.mock.calls[0][0] as Blob;
    expect(blob.type).toBe("text/calendar;charset=utf-8");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:ticket-calendar");
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
