import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiDocsPage } from "./pages";

describe("ApiDocsPage", () => {
  const swaggerUIBundle = Object.assign(
    vi.fn(() => ({})),
    {
      presets: {
        apis: "apis",
      },
    },
  );

  beforeEach(() => {
    swaggerUIBundle.mockClear();
    Object.defineProperty(window, "SwaggerUIBundle", {
      configurable: true,
      value: swaggerUIBundle,
    });
    Object.defineProperty(window, "SwaggerUIStandalonePreset", {
      configurable: true,
      value: "standalone",
    });
  });

  it("mounts Swagger UI against the bundled OpenAPI contract", async () => {
    render(<ApiDocsPage />);

    expect(screen.getByLabelText("API Contract")).toBeInTheDocument();
    expect(screen.getByLabelText("Swagger UI")).toBeInTheDocument();
    await waitFor(() =>
      expect(swaggerUIBundle).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "/openapi/openapi.bundle.yaml",
          dom_id: "#swagger-ui",
          layout: "StandaloneLayout",
          tryItOutEnabled: true,
        }),
      ),
    );
  });
});
