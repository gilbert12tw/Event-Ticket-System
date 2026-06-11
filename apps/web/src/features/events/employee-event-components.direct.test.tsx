import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
  EmployeeEventCard,
  EventSummaryBlock,
  FamilyCountControl,
  WaitlistPolicy,
  messageTone,
  registrationIDFor,
} from "./employee-event-components";
import { eventFixture } from "@/test/event-fixtures";

describe("employee event components", () => {
  it("renders bounded family controls for each capacity and registration state", async () => {
    const onChange = vi.fn();

    const { rerender } = render(
      <FamilyCountControl
        event={eventFixture({ capacity_type: "limited" })}
        value={0}
        onChange={onChange}
      />,
    );

    expect(
      screen.getByText("限量活動不開放填寫家屬人數。"),
    ).toBeInTheDocument();

    rerender(
      <FamilyCountControl
        event={eventFixture({
          allows_family: false,
          capacity: null,
          capacity_type: "unlimited",
          remaining_capacity: null,
        })}
        value={0}
        onChange={onChange}
      />,
    );
    expect(screen.getByText("此活動未開放攜帶家屬。")).toBeInTheDocument();

    rerender(
      <FamilyCountControl
        event={eventFixture({
          allows_family: true,
          capacity: null,
          capacity_type: "unlimited",
          current_user_status: "confirmed",
          current_user_ticket: {
            family_count: 3,
            registration_id: "reg-family",
            ticket_id: "ticket-family",
          },
          remaining_capacity: null,
        })}
        value={0}
        onChange={onChange}
      />,
    );
    expect(screen.getByText(/同行家屬：3 人/)).toBeInTheDocument();

    rerender(
      <FamilyCountControl
        event={eventFixture({
          allows_family: true,
          capacity: null,
          capacity_type: "unlimited",
          remaining_capacity: null,
        })}
        value={15}
        onChange={onChange}
      />,
    );

    const input = screen.getByRole("spinbutton", { name: "同行家屬" });
    expect(input).toHaveValue(10);
    await userEvent.clear(input);
    await userEvent.type(input, "12");
    expect(onChange).toHaveBeenLastCalledWith(10);
  });

  it("renders waitlist policy only when limited events need recovery copy", () => {
    const { rerender } = render(
      <WaitlistPolicy
        event={eventFixture({
          capacity: null,
          capacity_type: "unlimited",
          remaining_capacity: null,
          waitlist_count: 3,
        })}
      />,
    );
    expect(screen.queryByText("候補政策")).not.toBeInTheDocument();

    rerender(
      <WaitlistPolicy
        event={eventFixture({ remaining_capacity: 2, waitlist_count: 0 })}
      />,
    );
    expect(screen.queryByText("候補政策")).not.toBeInTheDocument();

    rerender(
      <WaitlistPolicy
        event={eventFixture({
          allocation_mode: "lottery",
          remaining_capacity: 0,
          waitlist_count: 2,
        })}
      />,
    );
    expect(screen.getByText("候補政策")).toBeInTheDocument();
    expect(screen.getByText(/抽籤或管理員釋出/)).toBeInTheDocument();

    rerender(
      <WaitlistPolicy
        event={eventFixture({
          allocation_mode: "fcfs",
          remaining_capacity: 1,
          waitlist_count: 2,
        })}
      />,
    );
    expect(screen.getByText(/先到先處理/)).toBeInTheDocument();
  });

  it("renders summary fallbacks, cooldown, and ineligible reasons", () => {
    render(
      <EventSummaryBlock
        event={eventFixture({
          description: "Phase 1 第一階段 示範 活動",
          eligibility: {
            eligible: false,
            no_show_cooldown: {
              active: true,
              until: "2026-06-20T10:00:00Z",
            },
            reasons: ["部門不符合", "職級不足"],
            warnings: [
              {
                code: "cross_city",
                employee_city: "Taipei",
                event_city: "Hsinchu",
                message: "跨城市活動",
              },
            ],
          },
          eligible: true,
          remaining_capacity: 0,
          waitlist_count: 2,
        })}
      />,
    );

    expect(screen.getByText("流程 活動")).toBeInTheDocument();
    expect(screen.getByText("不符合資格：")).toBeInTheDocument();
    expect(screen.getByText("部門不符合, 職級不足")).toBeInTheDocument();
    expect(screen.getByText("跨城市活動提醒")).toBeInTheDocument();
    expect(screen.getByText(/此活動位於 Hsinchu/)).toBeInTheDocument();
    expect(
      screen.getByText(/限量活動因缺席冷卻期暫停報名/),
    ).toBeInTheDocument();
    expect(screen.getByText("已額滿，可候補")).toBeInTheDocument();
  });

  it("renders card blocked, ticket, and registered cancellation states", () => {
    const onCancel = vi.fn();
    const onReasonChange = vi.fn();

    const { rerender } = render(
      <EmployeeEventCard
        event={eventFixture({
          eligible: false,
          eligibility_reason: "not eligible",
        })}
        mode="unavailable"
      />,
    );
    expect(screen.getByRole("button", { name: /不符合資格/ })).toBeDisabled();

    rerender(
      <EmployeeEventCard
        event={eventFixture({
          current_user_status: "confirmed",
          current_user_ticket: {
            registration_id: "reg-ticket",
            status: "active",
            ticket_id: "ticket-1",
          },
        })}
        mode="registered"
        onCancel={onCancel}
        onCancelReasonChange={onReasonChange}
      />,
    );
    expect(screen.getByRole("link", { name: /查看票券/ })).toHaveAttribute(
      "href",
      "/user/tickets?ticket_id=ticket-1",
    );
    expect(
      screen.getByRole("button", { name: "取消報名" }),
    ).toBeInTheDocument();
  });

  it("keeps small helper mappings deterministic", () => {
    expect(
      registrationIDFor(
        eventFixture({
          current_user_registration_id: "",
          current_user_ticket: {
            registration_id: "reg-from-ticket",
            ticket_id: "ticket-1",
          },
        }),
      ),
    ).toBe("reg-from-ticket");
    expect(registrationIDFor(eventFixture())).toBe("");
    expect(messageTone("booking failed because not eligible")).toBe("fail");
    expect(messageTone("候補成功")).toBe("ok");
    expect(messageTone("已額滿，進入候補")).toBe("warn");
    expect(messageTone("需要主辦確認")).toBe("info");
  });
});
