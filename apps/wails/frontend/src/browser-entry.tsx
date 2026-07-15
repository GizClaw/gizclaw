import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";

import type { RuntimeContext } from "./lib/runtime/types";
import { AdminFullHome } from "./views/admin/AdminFullHome";
import { PlayFullHome } from "./views/play/PlayFullHome";
import "./styles.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing root element");

function BrowserSurface() {
  const [runtime, setRuntime] = useState<RuntimeContext | null>(null);
  const [error, setError] = useState("");
  const surface = document.body.dataset.surface === "admin" ? "admin" : "play";
  useEffect(() => {
    if (window.__GIZCLAW_DESKTOP_TEST_RUNTIME__) {
      setRuntime(window.__GIZCLAW_DESKTOP_TEST_RUNTIME__);
      return;
    }
    const token = new URLSearchParams(window.location.hash.slice(1)).get("launch");
    if (!token) { setError("This launch link is missing or has already been consumed."); return; }
    history.replaceState(null, "", "/");
    fetch("/__gizclaw/runtime", {
      method: "POST",
      cache: "no-store",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    }).then(async (response) => {
      if (!response.ok) throw new Error(await response.text());
      return response.json() as Promise<RuntimeContext>;
    }).then((next) => {
      setRuntime(next);
    }).catch((reason) => setError(reason instanceof Error ? reason.message : String(reason)));
  }, []);
  if (error) return <div className="browser-launch-state"><h1>Unable to open {surface === "admin" ? "Admin" : "Play"}</h1><p>{error}</p></div>;
  if (!runtime) return <div className="browser-launch-state"><span className="browser-spinner" /><h1>Opening {surface === "admin" ? "Admin" : "Play"}</h1><p>Accepting the secure one-time desktop handoff…</p></div>;
  const close = async () => { window.close(); };
  return surface === "admin" ? <AdminBrowserSurface handoff={runtime} onClose={close} /> : <PlayFullHome onSignOut={close} runtime={runtime} />;
}

function AdminBrowserSurface({ handoff, onClose }: { handoff: RuntimeContext; onClose(): Promise<void> }) {
  const options = handoff.admin_servers ?? [];
  const initial = options.find((option) => option.id === handoff.admin_server_id) ?? options.find((option) => option.context.endpoint === handoff.context?.endpoint);
  const [selectedID, setSelectedID] = useState(initial?.id ?? options[0]?.id ?? "");
  const selected = options.find((option) => option.id === selectedID);
  const runtime: RuntimeContext = selected ? { context: selected.context, private_key_base64: selected.private_key_base64 } : handoff;
  return <><AdminFullHome key={selectedID || runtime.context?.endpoint} onSignOut={onClose} runtime={runtime} />{options.length > 1 ? <label className="admin-server-switch"><span>Server</span><select onChange={(event) => setSelectedID(event.target.value)} value={selectedID}>{options.map((option) => <option key={option.id} value={option.id}>{option.name}</option>)}</select></label> : null}</>;
}

createRoot(root).render(<React.StrictMode><BrowserSurface /></React.StrictMode>);

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_RUNTIME__?: RuntimeContext;
  }
}
