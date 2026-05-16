import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  archiveEvent,
  changeEventState,
  createEvent,
  duplicateEvent,
  listAdminEvents,
  seedDemo,
  updateEvent,
} from "@/lib/api";
import type { EventSummary } from "@/lib/api";
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
    seedDemo: vi.fn(),
    updateEvent: vi.fn(),
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
  });

  it("keeps list, edit, status, eligibility, and danger work in separate tabs", async () => {
    render(<AdminEventsPage />);

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
});

function eventFixture(overrides: Partial<EventSummary> = {}): EventSummary {
  return {
    event_id: "evt-1",
    title: "活動",
    description: "公司活動",
    location: "台北總部禮堂",
    event_city: "Taipei",
    event_site: "Taipei",
    starts_at: "2026-06-01T10:00:00Z",
    registration_start: "2026-05-01T10:00:00Z",
    registration_close: "2026-05-31T10:00:00Z",
    capacity_type: "limited",
    capacity: 10,
    allows_family: false,
    status: "published",
    allocation_mode: "fcfs",
    created_by: "admin-1",
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active",
    },
    eligible: true,
    eligibility_reason: "eligible",
    confirmed_count: 1,
    waitlist_count: 0,
    remaining_capacity: 9,
    current_user_status: "",
    no_show_cooldown: { active: false },
    ...overrides,
  };
}
