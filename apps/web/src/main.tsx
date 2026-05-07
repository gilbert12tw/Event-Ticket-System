import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles.css";
import "./layout-hardening.css";

const rootNode = document.getElementById("root");

if (!rootNode) {
  throw new Error("React root node is missing");
}

createRoot(rootNode).render(
  <StrictMode>
    <App />
  </StrictMode>
);
