import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { bookEvent, cancelMyRegistration, listEvents } from "@/lib/api";
import { EmployeeEventsPage } from "./employee-pages";
import { claims, eventFixture } from "./employee-pages-test-helpers";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    bookEvent: vi.fn(),
    cancelMyRegistration: vi.fn(),
    getEvent: vi.fn(),
    listEvents: vi.fn(),
  };
});

const mockListEvents = vi.mocked(listEvents);
const mockBookEvent = vi.mocked(bookEvent);
const mockCancelMyRegistration = vi.mocked(cancelMyRegistration);

describe("EmployeeEventsPage", () => {
  beforeEach(() => {
    mockListEvents.mockReset();
    mockBookEvent.mockReset();
    mockCancelMyRegistration.mockReset();
    window.history.replaceState({}, "", "/user/events");
  });

  it("renders a compact formal event list without debug identity panels", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        event_id: "evt-limited",
        title: "限量活動",
        capacity_type: "limited",
        capacity: 5,
        remaining_capacity: 3,
        current_user_status: "cancelled",
      }),
      eventFixture({
        event_id: "evt-unlimited",
        title: "不限量活動",
        capacity_type: "unlimited",
        capacity: null,
        remaining_capacity: null,
        allows_family: true,
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText("活動列表")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "可報名 1" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "我的報名 0" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "不可報名 1" })).toBeInTheDocument();
    expect(screen.getAllByText("符合資格").length).toBeGreaterThan(0);
    expect(screen.queryByText("員工入口")).not.toBeInTheDocument();
    expect(screen.queryByText("身分宣告")).not.toBeInTheDocument();
    expect(screen.queryByText("資格規則")).not.toBeInTheDocument();
    expect(screen.queryByText("已取消")).not.toBeInTheDocument();
    expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  });

  it("blocks cancelled registrations from appearing as bookable actions", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        current_user_status: "cancelled",
        event_id: "evt-cancelled",
        title: "已取消活動",
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("tab", { name: "不可報名 1" }),
    );

    const cancelledAction = await screen.findByRole("button", {
      name: /已取消/,
    });
    expect(cancelledAction).toBeDisabled();

    await userEvent.click(cancelledAction);

    expect(mockBookEvent).not.toHaveBeenCalled();
  });

  it("shows cooldown feedback and disables self-cancel after registration close", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        current_user_registration_id: "reg-closed",
        current_user_status: "confirmed",
        event_id: "evt-cooldown",
        no_show_cooldown: {
          active: true,
          applies_to: "limited",
          until: "2026-08-01T00:00:00Z",
          reason: "no_show_cooldown",
        },
        registration_close: "2020-01-01T00:00:00Z",
        title: "冷卻期活動",
        eligibility: {
          event_id: "evt-cooldown",
          eligible: true,
          can_book: false,
          reasons: [],
          warnings: [],
          no_show_cooldown: {
            active: true,
            until: "2026-08-01T00:00:00Z",
            reason: "no_show_cooldown",
          },
        },
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("tab", { name: "我的報名 1" }),
    );
    expect(
      await screen.findByText(/限量活動因缺席冷卻期暫停報名/),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /已報名/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: "取消報名" })).toBeDisabled();
    expect(screen.getByText(/自助取消已關閉/)).toBeInTheDocument();
  });

  it("renders cross-city warning and keeps booking link enabled when can_book=true", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        event_id: "evt-crosscity",
        title: "Hsinchu Event",
        event_city: "Hsinchu",
        eligibility: {
          event_id: "evt-crosscity",
          eligible: true,
          can_book: true,
          reasons: [],
          warnings: [
            {
              code: "cross_city",
              message:
                "This event is in Hsinchu; your registered city is Taipei.",
              employee_city: "Taipei",
              event_city: "Hsinchu",
            },
          ],
          no_show_cooldown: { active: false },
        },
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText(/跨城市活動提醒/)).toBeInTheDocument();
    expect(
      await screen.findByText(/此活動位於 Hsinchu，你的登錄城市為 Taipei。/),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /立即報名/ })).toBeInTheDocument();
  });

  it("disables booking and shows reason when can_book=false", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        event_id: "evt-ineligible",
        title: "Legal Event",
        eligibility: {
          event_id: "evt-ineligible",
          eligible: false,
          can_book: false,
          reasons: ["department does not match"],
          warnings: [],
          no_show_cooldown: { active: false },
        },
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("tab", { name: "不可報名 1" }),
    );
    expect(
      (await screen.findAllByText(/department does not match/)).length,
    ).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: /不符合資格/ })).toBeDisabled();
  });

  it("renders event without eligibility object without crashing", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        event_id: "evt-noelig",
        title: "No Eligibility Event",
        eligible: true,
        eligibility_reason: "eligible",
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText("No Eligibility Event")).toBeInTheDocument();
  });

  it("shows a disabled blocker action for unavailable event rows", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        eligible: false,
        eligibility_reason: "department Sales is not eligible",
        event_id: "evt-blocked",
        title: "不可報名活動",
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("tab", { name: "不可報名 1" }),
    );

    const blockedAction = await screen.findByRole("button", {
      name: /不符合資格/,
    });
    expect(blockedAction).toBeDisabled();
    expect(blockedAction).toHaveAttribute(
      "title",
      expect.stringContaining("不符合資格"),
    );
    expect(screen.getByRole("link", { name: "詳情" })).toBeInTheDocument();
  });

  it("opens event detail from list booking actions instead of submitting", async () => {
    const event = eventFixture({
      event_id: "evt-open",
      title: "開放報名活動",
      remaining_capacity: 2,
    });
    mockListEvents.mockResolvedValue([event]);

    render(<EmployeeEventsPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("link", { name: "立即報名" }),
    );

    expect(mockBookEvent).not.toHaveBeenCalled();
    expect(window.location.pathname).toBe("/user/events/detail");
    expect(window.location.search).toBe("?event_id=evt-open");
  });

  it("dismisses cancellation confirmation without calling the API", async () => {
    mockListEvents.mockResolvedValue([
      eventFixture({
        current_user_registration_id: "R-keep",
        current_user_status: "confirmed",
        title: "保留報名活動",
      }),
    ]);

    render(<EmployeeEventsPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("tab", { name: "我的報名 1" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "取消報名" }),
    );
    expect(await screen.findByRole("alertdialog")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "保留報名" }));

    expect(mockCancelMyRegistration).not.toHaveBeenCalled();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });
});
