import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, checkIn, reports } from "@/lib/api";
import { CheckinPage } from "./pages";

const zxingMocks = vi.hoisted(() => ({
  decodeFromConstraints: vi.fn(),
}));

vi.mock("@zxing/browser", () => ({
  BrowserQRCodeReader: class MockBrowserQRCodeReader {
    decodeFromConstraints = zxingMocks.decodeFromConstraints;
  },
}));

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    ApiError: actual.ApiError,
    checkIn: vi.fn(),
    listAdminEvents: vi.fn(),
    offlineCheckinPackage: vi.fn(),
    reports: vi.fn(),
    syncOfflineCheckins: vi.fn(),
  };
});

const mockCheckIn = vi.mocked(checkIn);
const mockReports = vi.mocked(reports);
const originalMediaDevices = navigator.mediaDevices;

describe("CheckinPage", () => {
  beforeEach(() => {
    window.history.pushState({}, "", "/admin/checkin");
    localStorage.clear();
    mockCheckIn.mockClear();
    mockReports.mockClear();
    zxingMocks.decodeFromConstraints.mockReset();
    mockCameraAvailable();
  });

  afterEach(() => {
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: originalMediaDevices,
    });
    vi.clearAllMocks();
  });

  it("prefills token from navigation state and ignores production localStorage tokens by default", async () => {
    mockReports.mockResolvedValue([]);
    localStorage.setItem("cets:lastTicketToken", "legacy-token");

    window.history.pushState(
      { cetsCheckinToken: "state-token" },
      "",
      "/admin/checkin",
    );
    render(<CheckinPage />);

    const tokenField = (await screen.findByLabelText(
      /掃描或貼上票券/,
    )) as HTMLInputElement;
    expect(tokenField.value).toBe("state-token");
    expect(
      screen.getByRole("button", { name: "使用最近票券" }),
    ).toHaveAttribute("disabled");
  });

  it("shows empty token input when no handoff source exists", async () => {
    window.history.pushState({}, "", "/admin/checkin");
    mockReports.mockResolvedValue([]);
    render(<CheckinPage />);

    const tokenField = (await screen.findByLabelText(
      /掃描或貼上票券/,
    )) as HTMLInputElement;
    expect(tokenField.value).toBe("");
    expect(
      screen.getByRole("button", { name: "使用最近票券" }),
    ).toHaveAttribute("disabled");
  });

  it("keeps accepted check-in visible when summary refresh is forbidden", async () => {
    mockReports.mockRejectedValue(
      new ApiError(403, {
        success: false,
        data: null,
        error: "role is not allowed",
      }),
    );
    mockCheckIn.mockResolvedValue({
      checkin_id: "chk-live",
      ticket_id: "tkt-live",
      event_id: "evt-live",
      employee_id: "E1001",
      status: "accepted",
      scanned_at: "2026-05-16T10:00:00Z",
      duplicate: false,
      holder: {
        display_name: "Ariel Chen",
        department: "Engineering",
        city: "Taipei",
      },
      family_count: 0,
    });

    render(<CheckinPage />);
    await userEvent.type(
      await screen.findByLabelText(/掃描或貼上票券/),
      "signed-token",
    );
    await userEvent.click(screen.getByRole("button", { name: "送出驗票" }));

    expect(mockCheckIn).toHaveBeenCalledWith("signed-token", "gate-1");
    expect(
      await screen.findByRole("heading", { name: "驗票成功" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("role is not allowed")).not.toBeInTheDocument();
  });

  it("keeps manual fallback available when camera access is unsupported", async () => {
    mockReports.mockResolvedValue([]);
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: undefined,
    });

    render(<CheckinPage />);
    await userEvent.click(
      await screen.findByRole("button", { name: "手機掃描 QR" }),
    );

    expect(screen.getByText(/此瀏覽器無法開啟相機/)).toBeInTheDocument();
    expect(screen.getByLabelText(/掃描或貼上票券/)).toBeInTheDocument();
  });

  it("fills the token field when QR detection reads a code", async () => {
    mockReports.mockResolvedValue([]);
    const controls = { stop: vi.fn() };
    zxingMocks.decodeFromConstraints.mockImplementation(
      async (_constraints, _video, callback) => {
        callback({ getText: () => "qr-token-123" });
        return controls;
      },
    );

    render(<CheckinPage />);
    await userEvent.click(
      await screen.findByRole("button", { name: "手機掃描 QR" }),
    );

    expect(
      await screen.findByText("已讀取 QR code，可以送出驗票。"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/掃描或貼上票券/)).toHaveValue("qr-token-123");
    expect(controls.stop).toHaveBeenCalledTimes(1);
  });

  it("opens the camera without depending on BarcodeDetector support", async () => {
    mockReports.mockResolvedValue([]);
    const controls = { stop: vi.fn() };
    zxingMocks.decodeFromConstraints.mockResolvedValue(controls);

    render(<CheckinPage />);
    await userEvent.click(
      await screen.findByRole("button", { name: "手機掃描 QR" }),
    );

    expect(
      await screen.findByText("相機已開啟，請將 QR code 對準畫面中央。"),
    ).toBeInTheDocument();
    expect(zxingMocks.decodeFromConstraints).toHaveBeenCalled();
  });

  it("shows camera permission failures without hiding manual entry", async () => {
    mockReports.mockResolvedValue([]);
    zxingMocks.decodeFromConstraints.mockRejectedValue(
      new DOMException("NotAllowed"),
    );

    render(<CheckinPage />);
    await userEvent.click(
      await screen.findByRole("button", { name: "手機掃描 QR" }),
    );

    expect(screen.getByText(/無法啟動相機/)).toBeInTheDocument();
    expect(screen.getByLabelText(/掃描或貼上票券/)).toBeInTheDocument();
  });

  it("stops scanner controls that resolve after the page unmounts", async () => {
    mockReports.mockResolvedValue([]);
    const controls = { stop: vi.fn() };
    let resolveControls: (controls: typeof controls) => void = () => {};
    const controlsPromise = new Promise<typeof controls>((resolve) => {
      resolveControls = resolve;
    });
    zxingMocks.decodeFromConstraints.mockReturnValue(controlsPromise);

    const { unmount } = render(<CheckinPage />);
    await userEvent.click(
      await screen.findByRole("button", { name: "手機掃描 QR" }),
    );
    unmount();

    await act(async () => {
      resolveControls(controls);
      await controlsPromise;
    });

    expect(controls.stop).toHaveBeenCalledTimes(1);
  });

  it("does not accept a QR result after the scanner is stopped", async () => {
    mockReports.mockResolvedValue([]);
    const controls = { stop: vi.fn() };
    let scanCallback: (result?: { getText: () => string }) => void = () => {};
    zxingMocks.decodeFromConstraints.mockImplementation(
      async (_constraints, _video, callback) => {
        scanCallback = callback;
        return controls;
      },
    );

    render(<CheckinPage />);
    await userEvent.click(
      await screen.findByRole("button", { name: "手機掃描 QR" }),
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "停止" })).not.toBeDisabled(),
    );
    await userEvent.click(screen.getByRole("button", { name: "停止" }));

    scanCallback({ getText: () => "late-token" });

    expect(screen.getByLabelText(/掃描或貼上票券/)).toHaveValue("");
    expect(controls.stop).toHaveBeenCalledTimes(1);
  });

  it("uses a full-width touch layout for mobile scanner actions", async () => {
    mockReports.mockResolvedValue([]);
    render(<CheckinPage />);

    expect(
      await screen.findByRole("button", { name: "手機掃描 QR" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "停止" })).toBeInTheDocument();
  });
});

function mockCameraAvailable() {
  Object.defineProperty(navigator, "mediaDevices", {
    configurable: true,
    value: {
      getUserMedia: vi.fn(),
    },
  });
}
