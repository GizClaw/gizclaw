import { Cpu, Gamepad2, Server } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Button } from "../components/Button";
import { Card, CardBody, CardHeader } from "../components/Card";
import { TextInput } from "../components/TextInput";
import { getDesktopAPI } from "../lib/runtime/desktop";
import type {
  BootstrapState,
  ContextSummary,
  CreateContextRequest,
  DesktopView,
  RuntimeContext,
} from "../lib/runtime/types";
import { AdminHome } from "../views/admin/AdminHome";
import { PlayHome } from "../views/play/PlayHome";

const emptyRuntime: RuntimeContext = {};

export function AppShell() {
  const api = useMemo(() => getDesktopAPI(), []);
  const [bootstrap, setBootstrap] = useState<BootstrapState | null>(null);
  const [runtime, setRuntime] = useState<RuntimeContext>(emptyRuntime);
  const [view, setView] = useState<DesktopView>("admin");
  const [error, setError] = useState<string>("");

  useEffect(() => {
    api
      .Bootstrap()
      .then((state) => {
        setBootstrap(state);
        setRuntime(state.runtime ?? emptyRuntime);
        setView(state.state.selected_view === "play" ? "play" : "admin");
      })
      .catch((err: unknown) => setError(errorMessage(err)));
  }, [api]);

  async function selectContext(name: string) {
    setError("");
    try {
      const next = await api.SelectContext(name);
      const contexts = await api.ListContexts();
      setRuntime(next);
      setBootstrap((prev) => (prev ? { ...prev, contexts, runtime: next } : prev));
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function createContext(req: CreateContextRequest) {
    setError("");
    try {
      const next = await api.CreateContext(req);
      const contexts = await api.ListContexts();
      setRuntime(next);
      setBootstrap((prev) =>
        prev
          ? {
              ...prev,
              contexts,
              runtime: next,
              state: { ...prev.state, selected_context: next.context?.name },
            }
          : prev,
      );
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function switchView(next: DesktopView) {
    setView(next);
    try {
      await api.SetSelectedView(next);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  const contexts = bootstrap?.contexts ?? [];
  const currentContext = runtime.context;

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark" />
          <span>GizClaw</span>
        </div>
        <nav>
          <button
            className={`nav-button ${view === "admin" ? "active" : ""}`}
            onClick={() => void switchView("admin")}
            type="button"
          >
            <Cpu size={17} /> Admin
          </button>
          <button
            className={`nav-button ${view === "play" ? "active" : ""}`}
            onClick={() => void switchView("play")}
            type="button"
          >
            <Gamepad2 size={17} /> Play
          </button>
        </nav>
      </aside>
      <section className="content">
        <header className="topbar">
          <div>
            <div className="topbar-title">{view === "admin" ? "Admin Console" : "Play Console"}</div>
            <div className="topbar-meta">{currentContext?.endpoint ?? "No context selected"}</div>
          </div>
          <div className="topbar-meta">
            {currentContext ? `Context: ${currentContext.name}` : "Select a context"}
          </div>
        </header>
        <main className="main">
          {error ? <div className="error">{error}</div> : null}
          <div className="grid-two">
            <ContextPicker contexts={contexts} current={currentContext} onSelect={selectContext} />
            <CreateContextForm onCreate={createContext} />
          </div>
          <RuntimeDetails runtime={runtime} />
          {view === "admin" ? <AdminHome runtime={runtime} /> : <PlayHome runtime={runtime} />}
        </main>
      </section>
    </div>
  );
}

function ContextPicker({
  contexts,
  current,
  onSelect,
}: {
  contexts: ContextSummary[];
  current?: ContextSummary;
  onSelect(name: string): Promise<void>;
}) {
  return (
    <Card>
      <CardHeader title="Contexts" />
      <CardBody>
        {contexts.length === 0 ? (
          <div className="empty">No contexts configured.</div>
        ) : (
          <div className="context-list">
            {contexts.map((ctx) => {
              const selected = current?.name === ctx.name || ctx.current;
              return (
                <button
                  className={`context-row ${selected ? "current" : ""}`}
                  key={ctx.name}
                  onClick={() => void onSelect(ctx.name)}
                  type="button"
                >
                  <div className="row-title">
                    <span>{ctx.name}</span>
                    {selected ? <span className="badge">current</span> : null}
                  </div>
                  <div className="row-meta">{ctx.description || ctx.endpoint}</div>
                  <div className="row-meta mono">{ctx.local_public_key}</div>
                </button>
              );
            })}
          </div>
        )}
      </CardBody>
    </Card>
  );
}

function CreateContextForm({ onCreate }: { onCreate(req: CreateContextRequest): Promise<void> }) {
  const [form, setForm] = useState<CreateContextRequest>({
    endpoint: "",
    name: "",
    server_public_key: "",
  });

  function update(key: keyof CreateContextRequest, value: string) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    void onCreate(form);
  }

  return (
    <Card>
      <CardHeader title="New Context" />
      <CardBody>
        <form className="form" onSubmit={submit}>
          <div className="field">
            <label htmlFor="context-name">Name</label>
            <TextInput id="context-name" onChange={(e) => update("name", e.target.value)} value={form.name} />
          </div>
          <div className="field">
            <label htmlFor="context-endpoint">Server endpoint</label>
            <TextInput
              id="context-endpoint"
              onChange={(e) => update("endpoint", e.target.value)}
              placeholder="127.0.0.1:9820"
              value={form.endpoint}
            />
          </div>
          <div className="field">
            <label htmlFor="context-server-key">Server public key</label>
            <TextInput
              id="context-server-key"
              onChange={(e) => update("server_public_key", e.target.value)}
              value={form.server_public_key}
            />
          </div>
          <div className="field">
            <label htmlFor="context-description">Description</label>
            <TextInput
              id="context-description"
              onChange={(e) => update("description", e.target.value)}
              value={form.description ?? ""}
            />
          </div>
          <Button type="submit">Create Context</Button>
        </form>
      </CardBody>
    </Card>
  );
}

function RuntimeDetails({ runtime }: { runtime: RuntimeContext }) {
  return (
    <Card>
      <CardHeader title="Runtime Injection" action={<Server size={17} />} />
      <CardBody>
        {runtime.context ? (
          <div className="details">
            <div className="detail-row">
              <div className="detail-key">Signaling URL</div>
              <div className="mono">{runtime.signaling_url}</div>
            </div>
            <div className="detail-row">
              <div className="detail-key">Server public key</div>
              <div className="mono">{runtime.context.server_public_key}</div>
            </div>
            <div className="detail-row">
              <div className="detail-key">Local public key</div>
              <div className="mono">{runtime.context.local_public_key}</div>
            </div>
            <div className="detail-row">
              <div className="detail-key">Private key</div>
              <div>Injected in memory</div>
            </div>
          </div>
        ) : (
          <div className="empty">Select or create a context to inject WebRTC runtime data.</div>
        )}
      </CardBody>
    </Card>
  );
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}
