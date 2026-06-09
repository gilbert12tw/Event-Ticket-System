import { expect, type Locator, type Page } from "@playwright/test";

export async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() => {
    const doc = document.documentElement;
    const body = document.body;
    const contentWidth = Math.max(
      doc.scrollWidth,
      doc.offsetWidth,
      doc.clientWidth,
      body.scrollWidth,
      body.offsetWidth,
      body.clientWidth,
      globalThis.innerWidth,
    );
    return Math.ceil(contentWidth - globalThis.innerWidth);
  });
  expect(overflow, "no horizontal overflow").toBeLessThanOrEqual(1);

  const contextLeaks = await page.evaluate(() =>
    Array.from(
      document.querySelectorAll<HTMLElement>(".workspace-context"),
    ).flatMap((context, index) => {
      const bounds = context.getBoundingClientRect();
      return Array.from(context.children)
        .filter(
          (child): child is HTMLElement =>
            child instanceof HTMLElement && child.offsetParent !== null,
        )
        .filter((child) => {
          const childBounds = child.getBoundingClientRect();
          return (
            childBounds.left < bounds.left - 1 ||
            childBounds.right > bounds.right + 1
          );
        })
        .map((child) => ({
          index,
          className: child.className,
          right: child.getBoundingClientRect().right,
          parentRight: bounds.right,
        }));
    }),
  );
  expect(
    contextLeaks,
    "workspace context children stay inside their panel",
  ).toEqual([]);

  const actionLeaks = await page.evaluate(() => {
    const selectors = [
      ".toolbar",
      ".row-actions",
      ".status-selectors",
      ".form-actions",
      ".section-heading",
    ];
    const leaks: Array<{
      index: number;
      selector: string | undefined;
      className: string;
    }> = [];
    const containers = Array.from(
      document.querySelectorAll<HTMLElement>(selectors.join(",")),
    );
    for (const [index, container] of containers.entries()) {
      const style = globalThis.getComputedStyle(container);
      if (container.offsetParent === null && style.position !== "fixed")
        continue;
      const bounds = container.getBoundingClientRect();
      if (bounds.width <= 0 || bounds.height <= 0) continue;
      const selector = selectors.find((candidate) =>
        container.matches(candidate),
      );
      for (const child of Array.from(container.children)) {
        if (!(child instanceof HTMLElement) || child.offsetParent === null) {
          continue;
        }
        const childBounds = child.getBoundingClientRect();
        if (
          childBounds.left < bounds.left - 1 ||
          childBounds.right > bounds.right + 1
        ) {
          leaks.push({
            index,
            selector,
            className: child.className,
          });
        }
      }
    }
    return leaks;
  });
  expect(
    actionLeaks,
    "toolbar and action children stay inside their containers",
  ).toEqual([]);

  const clippedButtons = await page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .filter((button) => button.offsetParent !== null)
      .filter(
        (button) =>
          button.scrollWidth > button.clientWidth + 1 ||
          button.scrollHeight > button.clientHeight + 1,
      )
      .map((button) => ({
        text:
          button.textContent?.trim() || button.getAttribute("aria-label") || "",
        className: button.className,
        scrollWidth: button.scrollWidth,
        clientWidth: button.clientWidth,
      })),
  );
  expect(clippedButtons, "visible buttons do not clip their text").toEqual([]);

  const apiLayout = await page.evaluate(() => {
    const shell = document.querySelector<HTMLElement>(".app-shell");
    const api = document.querySelector<HTMLElement>(".api-panel");
    if (!shell || !api) return null;
    return {
      columns: globalThis
        .getComputedStyle(shell)
        .gridTemplateColumns.split(" ")
        .filter(Boolean).length,
      position: globalThis.getComputedStyle(api).position,
      state: api.dataset.state,
      width: globalThis.innerWidth,
    };
  });
  if (apiLayout) {
    expect(apiLayout.state, "介接紀錄初始收合").toBe("collapsed");
  }
  if (apiLayout && apiLayout.width > 900 && apiLayout.width < 1680) {
    expect(
      apiLayout.columns,
      "收合後的介接紀錄不保留第三欄",
    ).toBeLessThanOrEqual(2);
  }

  const apiOverlaps = await page.evaluate(() => {
    const api = document.querySelector<HTMLElement>(".api-panel.collapsed");
    if (!api || api.offsetParent === null) return [];
    const apiBounds = api.getBoundingClientRect();
    const intersects = (target: DOMRect, source: DOMRect) =>
      target.left < source.right - 1 &&
      target.right > source.left + 1 &&
      target.top < source.bottom - 1 &&
      target.bottom > source.top + 1;
    return Array.from(
      document.querySelectorAll<HTMLElement>(
        [
          ".workspace button",
          ".workspace a",
          ".workspace tr",
          ".workspace .table-scroll",
          ".workspace .segmented-filter",
        ].join(","),
      ),
    )
      .filter((element) => element.offsetParent !== null)
      .filter((element) => !element.closest(".api-panel"))
      .filter((element) => {
        const bounds = element.getBoundingClientRect();
        if (bounds.width <= 0 || bounds.height <= 0) return false;
        if (bounds.bottom < 0 || bounds.top > globalThis.innerHeight)
          return false;
        return intersects(bounds, apiBounds);
      })
      .map((element) => ({
        tagName: element.tagName,
        text: element.textContent?.trim().slice(0, 32) || "",
        className: element.className,
      }));
  });
  expect(apiOverlaps, "收合後的介接紀錄不覆蓋表格列或操作按鈕").toEqual([]);

  const shellMode = await page.evaluate(() => {
    const width = globalThis.innerWidth;
    const desktop = document.querySelector<HTMLElement>(".desktop-sidebar");
    const topbar = document.querySelector<HTMLElement>(".mobile-topbar");
    const tabbar = document.querySelector<HTMLElement>(".mobile-tabbar");
    const workspace = document.querySelector<HTMLElement>(".workspace");
    const tabbarBounds = tabbar?.getBoundingClientRect();
    const tabTargets = Array.from(
      document.querySelectorAll<HTMLElement>(
        ".mobile-tabbar a, .mobile-tabbar button",
      ),
    ).map((target) => Math.round(target.getBoundingClientRect().height));
    return {
      width,
      desktopDisplay: desktop
        ? globalThis.getComputedStyle(desktop).display
        : "missing",
      topbarDisplay: topbar
        ? globalThis.getComputedStyle(topbar).display
        : "none",
      tabbarDisplay: tabbar
        ? globalThis.getComputedStyle(tabbar).display
        : "none",
      workspacePaddingBottom: workspace
        ? Number.parseFloat(
            globalThis.getComputedStyle(workspace).paddingBottom,
          )
        : 0,
      tabbarHeight: tabbarBounds ? Math.round(tabbarBounds.height) : 0,
      tabTargets,
    };
  });
  if (shellMode.width <= 900) {
    expect(shellMode.desktopDisplay, "mobile hides desktop sidebar").toBe(
      "none",
    );
    expect(shellMode.topbarDisplay, "mobile top bar visible").not.toBe("none");
    expect(shellMode.tabbarDisplay, "mobile bottom nav visible").not.toBe(
      "none",
    );
    expect(
      shellMode.workspacePaddingBottom,
      "workspace reserves bottom nav space",
    ).toBeGreaterThanOrEqual(shellMode.tabbarHeight);
    for (const height of shellMode.tabTargets) {
      expect(height, "mobile nav target height").toBeGreaterThanOrEqual(44);
    }
  } else {
    expect(shellMode.desktopDisplay, "desktop sidebar visible").not.toBe(
      "none",
    );
    expect(shellMode.topbarDisplay, "desktop hides mobile top bar").toBe(
      "none",
    );
    expect(shellMode.tabbarDisplay, "desktop hides mobile bottom nav").toBe(
      "none",
    );
  }
}

export async function expectPrimaryCtaTreatment(locator: Locator) {
  await expect(locator).toBeVisible();
  const styles = await locator.evaluate((element) => {
    const computed = globalThis.getComputedStyle(element);
    return {
      backgroundColor: computed.backgroundColor,
      color: computed.color,
    };
  });
  expect(styles.backgroundColor, "primary CTA keeps accent fill").not.toBe(
    "rgba(0, 0, 0, 0)",
  );
  expect(styles.color, "primary CTA keeps readable foreground").not.toBe(
    "oklch(0.3 0.022 245)",
  );
}

export async function expectElementAboveMobileTabbar(
  page: Page,
  selector: string,
) {
  const overlap = await page.evaluate((targetSelector) => {
    const target = document.querySelector<HTMLElement>(targetSelector);
    const tabbar = document.querySelector<HTMLElement>(".mobile-tabbar");
    if (!target || !tabbar) return null;
    const targetBounds = target.getBoundingClientRect();
    const tabbarBounds = tabbar.getBoundingClientRect();
    if (
      targetBounds.width <= 0 ||
      targetBounds.height <= 0 ||
      tabbarBounds.width <= 0 ||
      tabbarBounds.height <= 0 ||
      globalThis.getComputedStyle(tabbar).display === "none"
    ) {
      return null;
    }
    return {
      targetBottom: Math.round(targetBounds.bottom),
      tabbarTop: Math.round(tabbarBounds.top),
    };
  }, selector);
  if (overlap === null) return;
  expect(
    overlap.targetBottom,
    `${selector} stays above the mobile tab bar`,
  ).toBeLessThanOrEqual(overlap.tabbarTop);
}

export async function expectNotificationControlsCompact(page: Page) {
  const filterMetrics = await page.evaluate(() =>
    Array.from(
      document.querySelectorAll<HTMLButtonElement>(".segmented-filter-item"),
    ).map((button) => {
      const bounds = button.getBoundingClientRect();
      return {
        width: Math.round(bounds.width),
        height: Math.round(bounds.height),
      };
    }),
  );
  expect(
    filterMetrics.length,
    "notification delivery status filters are rendered",
  ).toBeGreaterThan(0);
  expect(
    filterMetrics,
    "notification delivery status filters stay compact",
  ).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        width: expect.any(Number),
        height: expect.any(Number),
      }),
    ]),
  );
  for (const metric of filterMetrics) {
    expect(metric.width, "status filter width").toBeLessThanOrEqual(160);
    expect(metric.height, "status filter height").toBeLessThanOrEqual(44);
  }
}
