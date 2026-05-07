import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const srcDir = join(process.cwd(), "src");

function filesUnder(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const fullPath = join(dir, entry);
    return statSync(fullPath).isDirectory() ? filesUnder(fullPath) : [fullPath];
  });
}

describe("frontend architecture boundaries", () => {
  it("keeps shadcn primitives in components/ui", () => {
    const uiFiles = filesUnder(join(srcDir, "components/ui"));

    expect(uiFiles.some((file) => file.endsWith("button.tsx"))).toBe(true);
    expect(uiFiles.every((file) => file.includes("components/ui"))).toBe(true);
  });

  it("keeps shared and layout components independent from feature modules", () => {
    const reusableFiles = [...filesUnder(join(srcDir, "components/shared")), ...filesUnder(join(srcDir, "components/layout"))];
    const offenders = reusableFiles.filter((file) => readFileSync(file, "utf8").includes("@/features/"));

    expect(offenders).toEqual([]);
  });
});
