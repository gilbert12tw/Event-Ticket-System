import { useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { defaultEventForm } from "@/lib/formatting";
import type { AdminCreateForm } from "./admin-event-crud-types";
import {
  EventCoreFields,
  IntentSelector,
  ReadinessGrid,
  TagInput,
} from "./admin-event-form-controls";

function FormHarness({
  initial,
}: Readonly<{ initial?: Partial<AdminCreateForm> }>) {
  const [form, setForm] = useState<AdminCreateForm>({
    ...defaultEventForm(),
    ...initial,
  });
  return (
    <>
      <EventCoreFields
        form={form}
        onChange={setForm}
        capacityReady={false}
        siteOptions={[
          { value: "", label: "未設定" },
          { value: "Taipei HQ", label: "台北總部" },
        ]}
        windowReady={false}
      />
      <output data-testid="capacity-type">{form.capacity_type}</output>
      <output data-testid="capacity">{form.capacity}</output>
    </>
  );
}

describe("admin event form controls", () => {
  it("keeps capacity invariants visible when switching limited and unlimited modes", async () => {
    const user = userEvent.setup();
    render(<FormHarness initial={{ capacity: "" }} />);

    expect(screen.getByLabelText(/活動開始/)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(screen.getByLabelText(/活動結束/)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(screen.getByLabelText(/報名開始/)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(screen.getByLabelText(/報名截止/)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(screen.getByLabelText(/容量/)).toHaveAttribute(
      "aria-invalid",
      "true",
    );

    await user.click(screen.getByRole("combobox", { name: "票數類型" }));
    await user.click(screen.getByRole("option", { name: "不限量" }));

    expect(screen.getByTestId("capacity-type")).toHaveTextContent("unlimited");
    expect(screen.getByTestId("capacity")).toHaveTextContent("");
    expect(screen.getByText(/不限量活動不設總名額/)).toBeInTheDocument();
    expect(screen.queryByLabelText(/容量/)).not.toBeInTheDocument();
  });

  it("adds and removes tags without emitting blank custom tags", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(<TagInput value="台北" onChange={onChange} />);

    await user.click(screen.getByRole("button", { name: "新增" }));
    expect(onChange).not.toHaveBeenCalled();

    await user.click(screen.getByRole("combobox", { name: "新增標籤" }));
    await user.click(screen.getByRole("option", { name: "家庭活動" }));
    expect(onChange).toHaveBeenLastCalledWith("台北, 家庭活動");

    await user.type(screen.getByLabelText("自訂標籤"), "內部活動");
    await user.click(screen.getByRole("button", { name: "新增" }));
    expect(onChange).toHaveBeenLastCalledWith("台北, 內部活動");

    const selectedTags = screen.getByLabelText("已選標籤");
    await user.click(
      within(selectedTags).getByRole("button", { name: "移除標籤 台北" }),
    );
    expect(onChange).toHaveBeenLastCalledWith("");
  });

  it("renders intent choices and readiness outcomes as accessible state", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <>
        <IntentSelector value="draft" onChange={onChange} />
        <ReadinessGrid
          checks={[
            { label: "時間設定完整", ok: true },
            { label: "容量設定完整", ok: false },
          ]}
        />
      </>,
    );

    expect(screen.getByLabelText("儲存草稿")).toBeChecked();
    await user.click(screen.getByLabelText("檢查通過後發布"));
    expect(onChange).toHaveBeenCalledWith("published");
    expect(screen.getByLabelText("發布檢查")).toHaveTextContent("時間設定完整");
    expect(screen.getByLabelText("發布檢查")).toHaveTextContent("通過");
    expect(screen.getByLabelText("發布檢查")).toHaveTextContent("需確認");
  });
});
