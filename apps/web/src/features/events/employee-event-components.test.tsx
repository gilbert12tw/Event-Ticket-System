import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Ticket } from "@/lib/api";
import {
  canBook,
  EmployeeEventCard,
  EventSummaryBlock,
  FamilyCountControl,
  messageTone,
  registrationIDFor,
  WaitlistPolicy,
} from "./employee-event-components";
import { eventFixture } from "@/test/event-fixtures";

function userTicket(overrides: Partial<Ticket> = {}): Ticket {
  return {
    ticket_id: "t-1",
    registration_id: "r-1",
    event_id: "evt-1",
    employee_id: "E1001",
    status: "active",
    issued_at: "2026-06-01T00:00:00Z",
    non_transferable: true,
    ...overrides,
  };
}

describe("EmployeeEventCard", () => {
  it("links the primary action to the detail page for bookable events", () => {
    render(<EmployeeEventCard event={eventFixture()} mode="available" />);

    const primary = screen.getByRole("link", { name: /立即報名/ });
    expect(primary).toHaveAttribute(
      "href",
      "/user/events/detail?event_id=evt-1",
    );
    expect(screen.getByText("9 席可報名")).toBeInTheDocument();
  });

  it("links to the ticket when one is already issued", () => {
    render(
      <EmployeeEventCard
        event={eventFixture({
          current_user_status: "confirmed",
          current_user_ticket: userTicket(),
        })}
        mode="registered"
      />,
    );

    expect(screen.getByRole("link", { name: /查看票券/ })).toHaveAttribute(
      "href",
      expect.stringContaining("t-1"),
    );
  });

  it("prevents primary navigation while a request is pending", async () => {
    const user = userEvent.setup();
    render(
      <EmployeeEventCard event={eventFixture()} mode="available" pending />,
    );

    const primary = screen.getByRole("link", { name: /立即報名/ });
    expect(primary).toHaveAttribute("aria-disabled", "true");
    await user.click(primary);
  });

  it("renders a disabled blocked action for ineligible events", () => {
    render(
      <EmployeeEventCard
        event={eventFixture({
          eligible: false,
          eligibility_reason: "department_mismatch",
        })}
        mode="unavailable"
      />,
    );

    expect(screen.getByRole("button", { name: /不符合資格/ })).toBeDisabled();
  });

  it("offers family setup for unlimited family-friendly events", () => {
    render(
      <EmployeeEventCard
        event={eventFixture({
          capacity_type: "unlimited",
          capacity: null as unknown as number,
          allows_family: true,
        })}
        mode="available"
      />,
    );

    expect(
      screen.getByRole("link", { name: /設定同行人數/ }),
    ).toBeInTheDocument();
  });

  it("shows cancellation control and cooldown alert when registered", () => {
    render(
      <EmployeeEventCard
        cancelReason=""
        event={eventFixture({
          current_user_status: "confirmed",
          no_show_cooldown: {
            active: true,
            applies_to: "limited",
            until: "2026-07-01T00:00:00Z",
          },
        })}
        mode="registered"
        onCancel={vi.fn()}
        onCancelReasonChange={vi.fn()}
      />,
    );

    expect(screen.getByText(/缺席冷卻期暫停報名/)).toBeInTheDocument();
  });
});

describe("EventSummaryBlock", () => {
  it("shows a capacity meter for limited events", () => {
    render(<EventSummaryBlock event={eventFixture({ confirmed_count: 4 })} />);

    expect(screen.getByText(/4\/10 已報名/)).toBeInTheDocument();
    expect(screen.getByText("限量")).toBeInTheDocument();
  });

  it("shows the no-stock hint for unlimited events", () => {
    render(
      <EventSummaryBlock
        event={eventFixture({
          capacity_type: "unlimited",
          capacity: null as unknown as number,
        })}
      />,
    );

    expect(screen.getByText(/不限量活動不扣庫存/)).toBeInTheDocument();
  });

  it("lists ineligible reasons from the eligibility decision", () => {
    render(
      <EventSummaryBlock
        event={eventFixture({
          eligibility: {
            event_id: "evt-1",
            eligible: false,
            can_book: false,
            reasons: ["部門不符"],
            warnings: [],
          },
        })}
      />,
    );

    expect(screen.getByText("不符合資格：")).toBeInTheDocument();
    expect(screen.getAllByText("部門不符").length).toBeGreaterThan(0);
  });

  it("falls back to a generic ineligible message without reasons", () => {
    render(
      <EventSummaryBlock
        event={eventFixture({
          eligibility: {
            event_id: "evt-1",
            eligible: false,
            can_book: false,
            reasons: [],
            warnings: [],
          },
        })}
      />,
    );

    expect(screen.getByText("目前不符合活動資格條件。")).toBeInTheDocument();
  });

  it("hides the event id in compact mode and warns during cooldown", () => {
    render(
      <EventSummaryBlock
        compact
        event={eventFixture({
          no_show_cooldown: {
            active: true,
            applies_to: "limited",
            until: "2026-07-01T00:00:00Z",
          },
        })}
      />,
    );

    expect(screen.queryByText("活動編號")).not.toBeInTheDocument();
    expect(screen.getByText(/缺席冷卻期暫停報名/)).toBeInTheDocument();
  });

  it("replaces an empty description with a default", () => {
    render(<EventSummaryBlock event={eventFixture({ description: "" })} />);

    expect(screen.getByText("未提供活動描述。")).toBeInTheDocument();
  });
});

describe("WaitlistPolicy", () => {
  it("renders nothing for unlimited events", () => {
    const { container } = render(
      <WaitlistPolicy event={eventFixture({ capacity_type: "unlimited" })} />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when seats remain and no one waits", () => {
    const { container } = render(<WaitlistPolicy event={eventFixture()} />);

    expect(container).toBeEmptyDOMElement();
  });

  it("explains the policy when the event is full", () => {
    render(
      <WaitlistPolicy
        event={eventFixture({ remaining_capacity: 0, waitlist_count: 3 })}
      />,
    );

    expect(screen.getByText("候補政策")).toBeInTheDocument();
    expect(screen.getByText(/先到先處理/)).toBeInTheDocument();
  });

  it("mentions lottery allocation when configured", () => {
    render(
      <WaitlistPolicy
        event={eventFixture({
          allocation_mode: "lottery",
          remaining_capacity: 2,
          waitlist_count: 1,
        })}
      />,
    );

    expect(screen.getByText(/抽籤或管理員釋出/)).toBeInTheDocument();
  });
});

describe("FamilyCountControl", () => {
  it("explains that limited events take no family members", () => {
    render(
      <FamilyCountControl
        event={eventFixture()}
        value={0}
        onChange={vi.fn()}
      />,
    );

    expect(
      screen.getByText("限量活動不開放填寫家屬人數。"),
    ).toBeInTheDocument();
  });

  it("explains when the event does not allow family", () => {
    render(
      <FamilyCountControl
        event={eventFixture({ capacity_type: "unlimited" })}
        value={0}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByText("此活動未開放攜帶家屬。")).toBeInTheDocument();
  });

  it("shows the recorded companion count after booking", () => {
    render(
      <FamilyCountControl
        event={eventFixture({
          capacity_type: "unlimited",
          allows_family: true,
          current_user_status: "confirmed",
          current_user_ticket: userTicket({ family_count: 2 }),
        })}
        value={0}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByText(/同行家屬：2 人/)).toBeInTheDocument();
  });

  it("clamps manual input to the allowed range", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <FamilyCountControl
        event={eventFixture({
          capacity_type: "unlimited",
          allows_family: true,
        })}
        value={99}
        onChange={onChange}
      />,
    );

    const input = screen.getByLabelText(/同行家屬/);
    expect(input).toHaveValue(10);

    await user.type(input, "3");
    expect(onChange).toHaveBeenCalled();
    expect(onChange.mock.calls.every(([next]) => next >= 0 && next <= 10)).toBe(
      true,
    );
  });
});

describe("helpers", () => {
  it("canBook follows the attendee action state", () => {
    expect(canBook(eventFixture())).toBe(true);
    expect(canBook(eventFixture({ eligible: false }))).toBe(false);
  });

  it("registrationIDFor prefers the registration id then the ticket", () => {
    expect(
      registrationIDFor(
        eventFixture({ current_user_registration_id: "reg-9" }),
      ),
    ).toBe("reg-9");
    expect(
      registrationIDFor(eventFixture({ current_user_ticket: userTicket() })),
    ).toBe("r-1");
    expect(registrationIDFor(eventFixture())).toBe("");
  });

  it.each([
    ["報名成功", "ok"],
    ["booking confirmed", "ok"],
    ["已額滿，加入候補", "warn"],
    ["waitlist position updated", "warn"],
    ["報名失敗", "fail"],
    ["not eligible for event", "fail"],
    ["其他訊息", "info"],
  ])("messageTone(%s) → %s", (message, tone) => {
    expect(messageTone(message)).toBe(tone);
  });
});
