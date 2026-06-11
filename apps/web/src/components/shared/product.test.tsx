import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  ActionBar,
  AppPageHeader,
  CompactStatsBar,
  DebugChromeGate,
  DebugToggle,
  EventListItem,
  HelperStrip,
  PageHeaderCompact,
  Surface,
} from "./product";

describe("DebugChromeGate", () => {
  it("renders children only when enabled", () => {
    const { rerender } = render(
      <DebugChromeGate enabled={false}>
        <span>debug tools</span>
      </DebugChromeGate>,
    );
    expect(screen.queryByText("debug tools")).not.toBeInTheDocument();

    rerender(
      <DebugChromeGate enabled>
        <span>debug tools</span>
      </DebugChromeGate>,
    );
    expect(screen.getByText("debug tools")).toBeInTheDocument();
  });
});

describe("Surface", () => {
  it("renders the requested element and variant class", () => {
    render(
      <Surface as="section" variant="danger">
        內容
      </Surface>,
    );

    const surface = screen.getByText("內容");
    expect(surface.tagName).toBe("SECTION");
    expect(surface.className).toContain("surface-danger");
  });
});

describe("ActionBar and HelperStrip", () => {
  it("applies alignment and tone classes", () => {
    render(
      <>
        <ActionBar align="end">
          <button type="button">動作</button>
        </ActionBar>
        <HelperStrip tone="warn">提示</HelperStrip>
      </>,
    );

    expect(
      screen.getByRole("button", { name: "動作" }).parentElement,
    ).toHaveClass("action-bar", "end");
    expect(screen.getByText("提示")).toHaveClass("helper-strip", "warn");
  });
});

describe("CompactStatsBar", () => {
  it("renders each stat under the given label", () => {
    render(
      <CompactStatsBar
        items={[
          { label: "已報名", value: 12 },
          { label: "候補", value: "3" },
        ]}
        label="活動統計"
      />,
    );

    const list = screen.getByLabelText("活動統計");
    expect(list).toContainElement(screen.getByText("已報名"));
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });
});

describe("AppPageHeader", () => {
  it("renders title, metrics, session, utilities, and actions", () => {
    render(
      <AppPageHeader
        actions={<button type="button">建立活動</button>}
        eyebrow="管理後台"
        icon="calendar"
        metrics={[{ label: "活動數", value: 5 }]}
        session={<span>session</span>}
        title="活動管理"
        utilities={<span>工具列</span>}
      />,
    );

    expect(
      screen.getByRole("heading", { level: 1, name: "活動管理" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("活動管理摘要")).toBeInTheDocument();
    expect(screen.getByText("工具列")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "建立活動" }),
    ).toBeInTheDocument();
  });

  it("omits metrics bar when there are no metrics", () => {
    render(
      <PageHeaderCompact
        eyebrow="員工"
        icon="calendar"
        metrics={[]}
        session={<span>session</span>}
        title="我的票券"
      />,
    );

    expect(screen.queryByLabelText("我的票券摘要")).not.toBeInTheDocument();
  });
});

describe("EventListItem", () => {
  it("renders badges, meta, description, and actions", () => {
    render(
      <EventListItem
        actions={<button type="button">報名</button>}
        badges={<span>限量</span>}
        description={<p>說明文字</p>}
        meta="6/12 · Taipei"
        title="夏季派對"
      />,
    );

    expect(
      screen.getByRole("heading", { name: "夏季派對" }),
    ).toBeInTheDocument();
    expect(screen.getByText("說明文字")).toBeInTheDocument();
  });

  it("omits the description container when absent", () => {
    const { container } = render(
      <EventListItem
        actions={<span />}
        badges={<span />}
        meta="meta"
        title="活動"
      />,
    );

    expect(
      container.querySelector(".event-list-item-description"),
    ).not.toBeInTheDocument();
  });
});

describe("DebugToggle", () => {
  it("flips the enabled state on click", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<DebugToggle enabled onToggle={onToggle} />);

    const toggle = screen.getByRole("button", { name: /Debug/ });
    expect(toggle).toHaveAttribute("aria-pressed", "true");

    await user.click(toggle);
    expect(onToggle).toHaveBeenCalledWith(false);
  });
});
