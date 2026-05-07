import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EmptyState, SkeletonRows, StatusBadge } from ".";

describe("shared product components", () => {
  it("renders status badges with semantic tone classes", () => {
    render(<StatusBadge tone="ok">confirmed</StatusBadge>);

    const badge = screen.getByText("confirmed");
    expect(badge).toHaveClass("status-badge", "ok");
  });

  it("renders empty and loading states without feature dependencies", () => {
    const { container } = render(
      <>
        <EmptyState title="尚無資料" action="建立活動後會出現在這裡。" />
        <SkeletonRows rows={3} />
      </>
    );

    expect(screen.getByText("尚無資料")).toBeInTheDocument();
    expect(screen.getByText("建立活動後會出現在這裡。")).toBeInTheDocument();
    expect(container.querySelectorAll(".skeleton-row")).toHaveLength(3);
  });
});
