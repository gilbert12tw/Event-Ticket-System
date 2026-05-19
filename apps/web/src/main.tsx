import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles.css";
import "./css/tokens.css";
import "./css/base.css";
import "./css/auth.css";
import "./css/shell.css";
import "./css/layout.css";
import "./css/forms-navigation.css";
import "./css/events.css";
import "./css/status-data.css";
import "./css/tickets-checkin.css";
import "./css/tables-admin.css";
import "./css/utilities.css";
import "./css/responsive.css";
import "./layout-hardening.css";
import "./layout-hardening-responsive.css";
import "./mobile-shell.css";

const rootNode = document.getElementById("root");

if (!rootNode) {
  throw new Error("React root node is missing");
}

createRoot(rootNode).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
