import userEvent from "@testing-library/user-event";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  Alert,
  AppPageHeader,
  CompactStatsBar,
  DebugChromeGate,
  EmptyState,
  EventListItem,
  Field,
  ResponsiveTable,
  SelectField,
  SkeletonRows,
  StatusBadge,
  TextareaField,
} from ".";
import { Button } from "@/components/ui/button";

describe("shared product components", () => {
  it("renders fields with labels, ids, names, descriptions, and invalid state", () => {
    render(
      <Field
        id="employee-id"
        label="員工編號"
        name="employee_id"
        value="E001"
        onChange={() => undefined}
        hint="必須使用公司員工編號。"
        invalid
      />,
    );

    const input = screen.getByLabelText("員工編號");
    expect(input).toHaveAttribute("id", "employee-id");
    expect(input).toHaveAttribute("name", "employee_id");
    expect(input).toHaveAttribute("autocomplete", "off");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("必須使用公司員工編號。");
  });

  it("renders select fields with trigger metadata and option helper text", async () => {
    render(
      <SelectField
        id="report-type"
        label="報表模板"
        name="report_type"
        value="participation"
        onChange={() => undefined}
        hint="請選擇匯出模板。"
        options={[
          { value: "", label: "選擇報表" },
          {
            value: "participation",
            label: "參與報表",
            helper: "適合人資週會。",
          },
        ]}
      />,
    );

    const trigger = screen.getByRole("combobox", { name: "報表模板" });
    expect(trigger).toHaveAttribute("id", "report-type");
    expect(trigger).toHaveAttribute("name", "report_type");
    expect(trigger).toHaveAccessibleDescription("請選擇匯出模板。");
    expect(trigger).toHaveTextContent("參與報表");
    expect(trigger).not.toHaveTextContent("適合人資週會。");

    await userEvent.click(trigger);
    const helper = await screen.findByText("適合人資週會。");
    expect(helper).toBeInTheDocument();
    expect(helper.closest("[data-slot='select-item']")).toHaveClass(
      "not-data-[variant=destructive]:hover:**:text-accent-foreground",
      "data-[highlighted]:text-accent-foreground",
    );
  });

  it("derives stable form names when operational controls omit name", () => {
    render(
      <>
        <Field
          id="state-reason"
          label="狀態原因"
          value=""
          onChange={() => undefined}
        />
        <TextareaField
          id="audit-note"
          label="稽核備註"
          value=""
          onChange={() => undefined}
        />
        <SelectField
          id="audit-role"
          label="角色"
          value=""
          onChange={() => undefined}
          options={[{ value: "", label: "所有角色" }]}
        />
      </>,
    );

    expect(screen.getByLabelText("狀態原因")).toHaveAttribute(
      "name",
      "state-reason",
    );
    expect(screen.getByLabelText("稽核備註")).toHaveAttribute(
      "name",
      "audit-note",
    );
    expect(screen.getByRole("combobox", { name: "角色" })).toHaveAttribute(
      "name",
      "audit-role",
    );
  });

  it("maps badges and alerts to the shared semantic tone classes", () => {
    const tones = ["ok", "warn", "fail", "info", "neutral"] as const;
    render(
      <>
        {tones.map((tone) => (
          <StatusBadge key={tone} tone={tone}>
            {tone}
          </StatusBadge>
        ))}
        {(["ok", "warn", "fail", "info"] as const).map((tone) => (
          <Alert key={tone} tone={tone}>
            {`${tone} alert`}
          </Alert>
        ))}
      </>,
    );

    for (const tone of tones) {
      expect(screen.getByText(tone)).toHaveClass("status-badge", tone);
    }
    for (const tone of ["ok", "warn", "fail", "info"] as const) {
      expect(screen.getByText(`${tone} alert`).closest("[role]")).toHaveClass(
        "app-alert",
        tone,
      );
    }
  });

  it("renders empty and loading states without feature dependencies", () => {
    const { container } = render(
      <>
        <EmptyState title="尚無資料" action="建立活動後會出現在這裡。" />
        <SkeletonRows rows={3} />
      </>,
    );

    expect(screen.getByText("尚無資料")).toBeInTheDocument();
    expect(screen.getByText("建立活動後會出現在這裡。")).toBeInTheDocument();
    expect(container.querySelectorAll(".skeleton-row")).toHaveLength(3);
  });

  it("hides debug chrome content until debug mode is enabled", () => {
    const { rerender } = render(
      <DebugChromeGate enabled={false}>
        <div>介接紀錄</div>
      </DebugChromeGate>,
    );

    expect(screen.queryByText("介接紀錄")).not.toBeInTheDocument();

    rerender(
      <DebugChromeGate enabled>
        <div>介接紀錄</div>
      </DebugChromeGate>,
    );
    expect(screen.getByText("介接紀錄")).toBeInTheDocument();
  });

  it("renders compact stats without KPI card surfaces", () => {
    const { container } = render(
      <CompactStatsBar
        items={[
          { label: "可報名", value: 2 },
          { label: "已報名", value: 1 },
        ]}
      />,
    );

    expect(screen.getByText("可報名")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(container.querySelector(".kpi")).not.toBeInTheDocument();
  });

  it("renders app page headers with utilities and compact metrics", () => {
    const { container } = render(
      <AppPageHeader
        eyebrow="員工工作區"
        icon="calendar"
        title="活動探索"
        session={<div aria-label="目前登入身份">Ariel Chen</div>}
        utilities={<Button type="button">Debug</Button>}
        metrics={[
          { label: "可報名", value: 2 },
          { label: "已報名", value: 1 },
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "活動探索" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("目前登入身份")).toHaveTextContent(
      "Ariel Chen",
    );
    expect(screen.getByRole("button", { name: "Debug" })).toBeInTheDocument();
    expect(container.querySelector(".kpi")).not.toBeInTheDocument();
  });

  it("renders compact event list items without detail metadata blocks", () => {
    const { container } = render(
      <EventListItem
        title="家庭電影夜"
        meta="05/19 上午11:25 · 台北總部"
        description="1 席可報名"
        badges={<StatusBadge tone="ok">符合資格</StatusBadge>}
        actions={<Button type="button">立即報名</Button>}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "家庭電影夜" }),
    ).toBeInTheDocument();
    expect(screen.getByText("05/19 上午11:25 · 台北總部")).toBeInTheDocument();
    expect(container.querySelector(".decision-strip")).not.toBeInTheDocument();
    expect(container.querySelector(".meta-list")).not.toBeInTheDocument();
  });

  it("renders responsive tables with accessible scroll container and caption", () => {
    render(
      <ResponsiveTable label="報名名單">
        <thead>
          <tr>
            <th>員工</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>王小明</td>
          </tr>
        </tbody>
      </ResponsiveTable>,
    );

    const region = screen.getByLabelText("報名名單");
    expect(within(region).getByRole("table")).toBeInTheDocument();
    expect(within(region).getByText("報名名單")).toHaveClass("sr-only");
  });
});
