import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  type Ticket,
  bookEvent,
  cancelMyRegistration,
  listEvents,
  listTickets,
} from "@/lib/api";
import {
  canBook,
  messageTone,
  registrationIDFor,
} from "./employee-event-components";
import { EmployeeEventsPage } from "./employee-pages";
import { claims, eventFixture } from "@/test/event-fixtures";

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    bookEvent: vi.fn(),
    cancelMyRegistration: vi.fn(),
    getEvent: vi.fn(),
    listEvents: vi.fn(),
    listTickets: vi.fn(),
  };
});

const mockListEvents = vi.mocked(listEvents);
const mockListTickets = vi.mocked(listTickets);
const mockBookEvent = vi.mocked(bookEvent);
const mockCancelMyRegistration = vi.mocked(cancelMyRegistration);

type EventOverrides = Parameters<typeof eventFixture>[0];

function ticket(
  ticket_id: string,
  registration_id = ticket_id.replace("T", "R"),
): Ticket {
  return {
    employee_id: "E1001",
    event_id: "evt-1",
    issued_at: "2026-05-06T10:00:00Z",
    non_transferable: true,
    registration_id,
    status: "active",
    ticket_id,
  };
}

function showEvents(...events: EventOverrides[]) {
  mockListEvents.mockResolvedValue(events.map((event) => eventFixture(event)));
}

describe("EmployeeEventsPage", () => {
  beforeEach(() => {
    mockListEvents.mockReset();
    mockListTickets.mockReset();
    mockListTickets.mockResolvedValue([]);
    mockBookEvent.mockReset();
    mockCancelMyRegistration.mockReset();
    window.history.replaceState({}, "", "/user/events");
  });

  it("renders a compact formal event list without debug identity panels", async () => {
    showEvents(
      {
        event_id: "evt-limited",
        title: "限量活動",
        capacity_type: "limited",
        capacity: 5,
        remaining_capacity: 3,
        current_user_status: "cancelled",
      },
      {
        event_id: "evt-unlimited",
        title: "不限量活動",
        capacity_type: "unlimited",
        capacity: null,
        remaining_capacity: null,
        allows_family: true,
      },
    );

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
    showEvents({
      current_user_status: "cancelled",
      event_id: "evt-cancelled",
      title: "已取消活動",
    });

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
    showEvents({
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
    });

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
    showEvents({
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
    });

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText(/跨城市活動提醒/)).toBeInTheDocument();
    expect(
      await screen.findByText(/此活動位於 Hsinchu，你的登錄城市為 Taipei。/),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /立即報名/ })).toBeInTheDocument();
  });

  it("disables booking and shows reason when can_book=false", async () => {
    showEvents({
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
    });

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
    showEvents({
      event_id: "evt-noelig",
      title: "No Eligibility Event",
      eligible: true,
      eligibility_reason: "eligible",
    });

    render(<EmployeeEventsPage claims={claims} />);

    expect(await screen.findByText("No Eligibility Event")).toBeInTheDocument();
  });

  it("classifies reusable event action helpers", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-05-20T00:00:00Z"));

    try {
      expect(messageTone("booking confirmed")).toBe("ok");
      expect(messageTone("報名已取消")).toBe("ok");
      expect(messageTone("waitlist closed")).toBe("warn");
      expect(messageTone("冷卻中")).toBe("warn");
      expect(messageTone("failed: not eligible")).toBe("fail");
      expect(messageTone("不符合資格")).toBe("fail");
      expect(messageTone("pending review")).toBe("info");

      expect(
        registrationIDFor(
          eventFixture({ current_user_registration_id: "R-direct" }),
        ),
      ).toBe("R-direct");
      expect(
        registrationIDFor(
          eventFixture({
            current_user_registration_id: "",
            current_user_ticket: ticket("T-ticket", "R-ticket"),
          }),
        ),
      ).toBe("R-ticket");
      expect(
        registrationIDFor(
          eventFixture({
            current_user_registration_id: "",
            current_user_ticket: undefined,
          }),
        ),
      ).toBe("");

      expect(canBook(eventFixture({ remaining_capacity: 1 }))).toBe(true);
      expect(canBook(eventFixture({ remaining_capacity: 0 }))).toBe(true);
      expect(canBook(eventFixture({ status: "draft" }))).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows a disabled blocker action for unavailable event rows", async () => {
    showEvents({
      eligible: false,
      eligibility_reason: "department Sales is not eligible",
      event_id: "evt-blocked",
      title: "不可報名活動",
    });

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
    showEvents({
      event_id: "evt-open",
      title: "開放報名活動",
      remaining_capacity: 2,
    });

    render(<EmployeeEventsPage claims={claims} />);

    await userEvent.click(
      await screen.findByRole("link", { name: "立即報名" }),
    );

    expect(mockBookEvent).not.toHaveBeenCalled();
    expect(window.location.pathname).toBe("/user/events/detail");
    expect(window.location.search).toBe("?event_id=evt-open");
  });

  it("dismisses cancellation confirmation without calling the API", async () => {
    showEvents({
      current_user_registration_id: "R-keep",
      current_user_status: "confirmed",
      title: "保留報名活動",
    });

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
