import { useEffect, useState } from "react";
import { Alert } from "@/components/shared";

const SWAGGER_UI_DOM_ID = "swagger-ui";
const OPENAPI_BUNDLE_URL = "/openapi/openapi.bundle.yaml";
const SWAGGER_UI_ASSET_BASE = "/swagger-ui";
const SWAGGER_UI_CSS_ID = "swagger-ui-css";

type SwaggerUIBundleFactory = ((options: {
  url: string;
  dom_id: string;
  presets: unknown[];
  layout: string;
  docExpansion: string;
  defaultModelsExpandDepth: number;
  tryItOutEnabled: boolean;
  persistAuthorization: boolean;
}) => unknown) & {
  presets: {
    apis: unknown;
  };
};

declare global {
  interface Window {
    SwaggerUIBundle?: SwaggerUIBundleFactory;
    SwaggerUIStandalonePreset?: unknown;
  }
}

const loadedScripts = new Map<string, Promise<void>>();

export function ApiDocsPage() {
  const [assetError, setAssetError] = useState("");

  useEffect(() => {
    let cancelled = false;

    ensureSwaggerUIAssets()
      .then(() => {
        if (cancelled) return;
        const { SwaggerUIBundle, SwaggerUIStandalonePreset } = window;
        if (!SwaggerUIBundle || !SwaggerUIStandalonePreset) {
          throw new Error("Swagger UI assets loaded without exposing globals.");
        }

        SwaggerUIBundle({
          url: OPENAPI_BUNDLE_URL,
          dom_id: `#${SWAGGER_UI_DOM_ID}`,
          presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
          layout: "StandaloneLayout",
          docExpansion: "none",
          defaultModelsExpandDepth: 1,
          tryItOutEnabled: true,
          persistAuthorization: false,
        });
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setAssetError(
            error instanceof Error
              ? error.message
              : "Unable to load Swagger UI assets.",
          );
        }
      });

    return () => {
      cancelled = true;
      document.getElementById(SWAGGER_UI_DOM_ID)?.replaceChildren();
    };
  }, []);

  return (
    <main className="api-docs-page" aria-label="API Contract">
      {assetError && <Alert tone="warn">{assetError}</Alert>}
      <div className="api-docs-host" aria-label="Swagger UI">
        <div id={SWAGGER_UI_DOM_ID} />
      </div>
    </main>
  );
}

function ensureSwaggerUIAssets() {
  ensureSwaggerUIStylesheet();
  if (window.SwaggerUIBundle && window.SwaggerUIStandalonePreset)
    return Promise.resolve();

  return loadScript(`${SWAGGER_UI_ASSET_BASE}/swagger-ui-bundle.js`).then(() =>
    loadScript(`${SWAGGER_UI_ASSET_BASE}/swagger-ui-standalone-preset.js`),
  );
}

function ensureSwaggerUIStylesheet() {
  if (document.getElementById(SWAGGER_UI_CSS_ID)) return;

  const link = document.createElement("link");
  link.id = SWAGGER_UI_CSS_ID;
  link.rel = "stylesheet";
  link.href = `${SWAGGER_UI_ASSET_BASE}/swagger-ui.css`;
  document.head.appendChild(link);
}

function loadScript(src: string) {
  const existing = loadedScripts.get(src);
  if (existing) return existing;

  const promise = new Promise<void>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = src;
    script.async = true;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error(`Unable to load ${src}`));
    document.head.appendChild(script);
  });

  loadedScripts.set(src, promise);
  return promise;
}
