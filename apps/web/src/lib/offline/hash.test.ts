import { describe, expect, it } from "vitest";
import { hashToken } from "./hash";

describe("hashToken", () => {
  it("produces correct SHA-256 hex for known input", async () => {
    const result = await hashToken("hello");
    expect(result).toBe(
      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
    );
  });

  it("produces correct hash for empty string", async () => {
    const result = await hashToken("");
    expect(result).toBe(
      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    );
  });

  it("produces lowercase hex", async () => {
    const result = await hashToken("test");
    expect(result).toMatch(/^[0-9a-f]{64}$/);
  });
});
