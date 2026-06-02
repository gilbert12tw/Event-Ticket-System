import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  archiveEvent,
  changeEventState,
  createEvent,
  duplicateEvent,
  listAdminEvents,
  previewEligibility,
  seedDemo,
  updateEvent,
  updateEligibility,
} from "@/lib/api";
import { eventFixture } from "@/test/event-fixtures";
import { AdminEventsPage } from "./admin-pages";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    archiveEvent: vi.fn(),
    changeEventState: vi.fn(),
    createEvent: vi.fn(),
    duplicateEvent: vi.fn(),
    listAdminEvents: vi.fn(),
    previewEligibility: vi.fn(),
    seedDemo: vi.fn(),
    updateEvent: vi.fn(),
    updateEligibility: vi.fn(),
  };
});

describe("AdminEventsPage CRUD tabs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.pushState({}, "", "/admin/events");
    vi.mocked(listAdminEvents).mockResolvedValue([eventFixture()]);
    vi.mocked(seedDemo).mockResolvedValue({ status: "seeded" });
    vi.mocked(createEvent).mockResolvedValue(
      eventFixture({ event_id: "evt-2" }),
    );
    vi.mocked(updateEvent).mockResolvedValue(eventFixture());
    vi.mocked(changeEventState).mockResolvedValue(eventFixture());
    vi.mocked(duplicateEvent).mockResolvedValue(
      eventFixture({ event_id: "evt-copy", title: "活動 copy" }),
    );
    vi.mocked(archiveEvent).mockResolvedValue(
      eventFixture({ status: "archived" }),
    );
    vi.mocked(previewEligibility).mockResolvedValue({
      event_id: "evt-1",
      match_count: 2,
      zero_match: false,
    });
    vi.mocked(updateEligibility).mockResolvedValue({
      event_id: "evt-1",
      match_count: 2,
      zero_match: false,
    });
  });

  it("keeps list, edit, status, eligibility, and danger work in separate tabs", async () => {
    const { container } = render(<AdminEventsPage />);

    expect(
      await screen.findByRole("tab", { name: "活動清單" }),
    ).toHaveAttribute("aria-selected", "true");
    expect(
      screen.queryByRole("button", { name: "儲存活動" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "封存活動" }),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "編輯活動" }));
    expect(
      await screen.findByRole("heading", { name: "編輯活動" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "儲存活動" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "封存活動" }),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "發布狀態" }));
    expect(
      await screen.findByRole("heading", { name: "發布狀態" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "更新狀態" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("發布狀態摘要")).toBeInTheDocument();
    expect(container.querySelector(".event-status-workspace .kpi")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "儲存活動" }),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "危險操作" }));
    expect(
      await screen.findByRole("heading", { name: "危險操作" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "封存活動" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "儲存活動" }),
    ).not.toBeInTheDocument();
  });

  it("restores the active CRUD tab from the URL query", async () => {
    window.history.pushState({}, "", "/admin/events?tab=danger");

    render(<AdminEventsPage />);

    await waitFor(() =>
      expect(screen.getByRole("tab", { name: "危險操作" })).toHaveAttribute(
        "aria-selected",
        "true",
      ),
    );
    expect(
      screen.getByRole("button", { name: "封存活動" }),
    ).toBeInTheDocument();
  });

  it("keeps the create form and result panel in one explicit workspace", async () => {
    const { container } = render(<AdminEventsPage />);

    await userEvent.click(await screen.findByRole("tab", { name: "建立活動" }));

    const workspace = container.querySelector(".create-workspace");
    expect(workspace).not.toBeNull();
    expect(workspace?.querySelector(".form-grid")).not.toBeNull();
    expect(workspace?.querySelector(".panel")).not.toBeNull();
  });

  it("updates the quick-setup template dropdown label after selection", async () => {
    render(<AdminEventsPage />);

    await userEvent.click(await screen.findByRole("tab", { name: "建立活動" }));

    const templateTrigger = screen.getByRole("combobox", {
      name: "活動模板",
    });
    expect(templateTrigger).toHaveTextContent("選擇模板");

    await userEvent.click(templateTrigger);
    await userEvent.click(
      await screen.findByRole("option", { name: /學習課程/ }),
    );

    await waitFor(() => expect(templateTrigger).toHaveTextContent("學習課程"));
    expect(screen.getByLabelText(/活動名稱/)).toHaveValue("內部學習工作坊");
  });

  it("updates the quick-setup schedule dropdown label after selection", async () => {
    render(<AdminEventsPage />);

    await userEvent.click(await screen.findByRole("tab", { name: "建立活動" }));

    const scheduleTrigger = screen.getByRole("combobox", {
      name: "排程預設",
    });
    expect(scheduleTrigger).toHaveTextContent("自訂時間");

    await userEvent.click(scheduleTrigger);
    await userEvent.click(
      await screen.findByRole("option", {
        name: /立即開放，活動前 24 小時截止/,
      }),
    );

    await waitFor(() =>
      expect(scheduleTrigger).toHaveTextContent("立即開放，活動前 24 小時截止"),
    );
  });

  it("submits create, duplicate, and archive actions from their workspaces", async () => {
    render(<AdminEventsPage />);

    await userEvent.click(await screen.findByRole("tab", { name: "建立活動" }));
    await userEvent.click(screen.getByRole("button", { name: "建立並發布" }));

    await waitFor(() =>
      expect(createEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "台北家庭電影夜",
          capacity_type: "limited",
          tags: ["家庭活動", "台北"],
          rule: expect.objectContaining({
            department: "Engineering",
            site: "Taipei",
          }),
        }),
      ),
    );
    const createPayload = vi.mocked(createEvent).mock.calls[0]?.[0];
    expect(createPayload).not.toHaveProperty("_selectedTemplate");
    expect(createPayload).not.toHaveProperty("_selectedSchedule");
    expect(
      await screen.findByText("活動已建立並寫入稽核紀錄。"),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "危險操作" }));
    await userEvent.click(screen.getByRole("button", { name: "複製活動" }));
    await waitFor(() => expect(duplicateEvent).toHaveBeenCalledWith("evt-1"));
    expect(
      await screen.findByText("已複製活動：活動 copy"),
    ).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("輸入活動名稱才能封存"), "活動");
    await userEvent.click(screen.getByRole("button", { name: "封存活動" }));
    await waitFor(() => expect(archiveEvent).toHaveBeenCalledWith("evt-1"));
    expect(await screen.findByText("活動已封存：活動")).toBeInTheDocument();
  });

  it("requires explicit confirmation before saving a zero-match eligibility rule", async () => {
    vi.mocked(previewEligibility).mockResolvedValueOnce({
      event_id: "evt-1",
      match_count: 0,
      zero_match: true,
    });
    vi.mocked(updateEligibility).mockResolvedValueOnce({
      event_id: "evt-1",
      match_count: 0,
      zero_match: true,
    });

    render(<AdminEventsPage />);

    await userEvent.click(await screen.findByRole("tab", { name: "資格預覽" }));
    await userEvent.click(
      screen.getByRole("button", { name: "Preview Impact" }),
    );

    expect(await screen.findByText(/命中 0 人/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Save Rule Version" }),
    ).toBeDisabled();

    await userEvent.click(
      screen.getByLabelText("我確認此規則可以儲存為 0 人命中版本"),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Save Rule Version" }),
    );

    await waitFor(() =>
      expect(updateEligibility).toHaveBeenCalledWith(
        "evt-1",
        expect.objectContaining({ allow_zero_match: true }),
      ),
    );
  });
});
