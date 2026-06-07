import { describe, expect, it } from "vitest";

import { buildQuerySuffix } from "./http";

describe("buildQuerySuffix", () => {
  it("serializes query params without object default stringification", () => {
    const query = buildQuerySuffix({
      search: " workshop ",
      page: 2,
      include_cancelled: false,
      empty: " ",
      tags: ["onsite", " internal "],
      filters: { site: "Taipei HQ" },
    });

    expect(query).toBe(
      "?search=workshop&page=2&include_cancelled=false&tags=onsite&tags=internal&filters=%7B%22site%22%3A%22Taipei+HQ%22%7D",
    );
  });
});
