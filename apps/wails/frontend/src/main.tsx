import React from "react";
import { createRoot } from "react-dom/client";
import { AppShell } from "./shell/AppShell";
import "./styles.css";

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing root element");
}

createRoot(root).render(
  <React.StrictMode>
    <AppShell />
  </React.StrictMode>,
);
