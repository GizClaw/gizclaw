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
  DesktopViewId,
  RuntimeContext,
} from "../lib/runtime/types";
import { AdminFullHome } from "../views/admin/AdminFullHome";
import { PlayFullHome } from "../views/play/PlayFullHome";

const emptyRuntime: RuntimeContext = {};

export function AppShell() {
  const api = useMemo(() => getDesktopAPI(), []);
  const [bootstrap, setBootstrap] = useState<BootstrapState | null>(null);
  const [runtime, setRuntime] = useState<RuntimeContext>(emptyRuntime);
  const [selectedContext, setSelectedContext] = useState<string>("");
  const [selectedView, setSelectedView] = useState<DesktopViewId>("admin");
  const [activeView, setActiveView] = useState<DesktopViewId | null>(null);
  const [connectingView, setConnectingView] = useState<DesktopViewId | null>(null);
  const [error, setError] = useState<string>("");

  useEffect(() => {
    api
      .Bootstrap()
      .then((state) => {
        setBootstrap(state);
        setSelectedContext(state.state.last_context ?? state.contexts.find((ctx) => ctx.current)?.name ?? state.contexts[0]?.name ?? "");
        setSelectedView(state.state.last_view === "play" ? "play" : "admin");
        setRuntime(emptyRuntime);
        setActiveView(null);
      })
      .catch((err: unknown) => setError(errorMessage(err)));
  }, [api]);

  async function refreshBootstrap() {
    const next = await api.Bootstrap();
    setBootstrap(next);
    return next;
  }

  async function selectContext(name: string) {
    setError("");
    try {
      await api.SelectContext(name);
      const next = await refreshBootstrap();
      setSelectedContext(name);
      setSelectedView(next.state.last_view === "play" ? "play" : selectedView);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function createContext(req: CreateContextRequest) {
    setError("");
    try {
      const context = await api.CreateContext(req);
      await refreshBootstrap();
      setSelectedContext(context.name);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function getStarted() {
    setError("");
    if (!selectedContext) {
      setError("Select a context first.");
      return;
    }
    setConnectingView(selectedView);
    try {
      await api.StartViewSession({ context_name: selectedContext, view: selectedView });
      const injected = await api.InjectedRuntime();
      setRuntime(injected);
      setActiveView(selectedView);
      await refreshBootstrap();
    } catch (err) {
      setError(errorMessage(err));
      setRuntime(emptyRuntime);
      setActiveView(null);
    } finally {
      setConnectingView(null);
    }
  }

  async function signOut() {
    setError("");
    try {
      await api.EndViewSession();
      await refreshBootstrap();
      setRuntime(emptyRuntime);
      setActiveView(null);
      setConnectingView(null);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  if (connectingView != null) {
    return <LoadingPage error={error} title={`Opening ${connectingView === "admin" ? "Admin" : "Play"}`} />;
  }

  if (activeView == null) {
    return (
      <WelcomePage
        bootstrap={bootstrap}
        error={error}
        onCreateContext={createContext}
        onGetStarted={getStarted}
        onSelectContext={selectContext}
        onSelectView={setSelectedView}
        selectedContext={selectedContext}
        selectedView={selectedView}
      />
    );
  }

  return activeView === "admin" ? (
    <AdminFullHome onSignOut={signOut} runtime={runtime} />
  ) : (
    <PlayFullHome onSignOut={signOut} runtime={runtime} />
  );
}

function WelcomePage({
  bootstrap,
  error,
  onCreateContext,
  onGetStarted,
  onSelectContext,
  onSelectView,
  selectedContext,
  selectedView,
}: {
  bootstrap: BootstrapState | null;
  error: string;
  onCreateContext(req: CreateContextRequest): Promise<void>;
  onGetStarted(): Promise<void>;
  onSelectContext(name: string): Promise<void>;
  onSelectView(view: DesktopViewId): void;
  selectedContext: string;
  selectedView: DesktopViewId;
}) {
  const contexts = bootstrap?.contexts ?? [];
  const views = bootstrap?.views ?? defaultViews();

  return (
    <div className="welcome-shell">
      <section className="welcome-panel">
        <div className="brand welcome-brand">
          <div className="brand-mark" />
          <span>GizClaw Desktop</span>
        </div>
        <div className="welcome-grid">
          <ContextPicker contexts={contexts} current={selectedContext} onSelect={onSelectContext} />
          <ViewPicker onSelect={onSelectView} selected={selectedView} views={views} />
        </div>
        <div className="welcome-actions">
          {error ? <div className="error">{error}</div> : null}
          <Button disabled={!selectedContext} onClick={() => void onGetStarted()} type="button">
            Get Started
          </Button>
        </div>
        <CreateContextForm onCreate={onCreateContext} />
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
  current: string;
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
              const selected = current === ctx.name;
              return (
                <button
                  className={`context-row ${selected ? "current" : ""}`}
                  key={ctx.name}
                  onClick={() => void onSelect(ctx.name)}
                  type="button"
                >
                  <div className="row-title">
                    <span>{ctx.name}</span>
                    {selected ? <span className="badge">selected</span> : null}
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

function ViewPicker({
  onSelect,
  selected,
  views,
}: {
  onSelect(view: DesktopViewId): void;
  selected: DesktopViewId;
  views: DesktopView[];
}) {
  return (
    <Card>
      <CardHeader title="Views" />
      <CardBody>
        <div className="context-list">
          {views.map((view) => {
            const isSelected = selected === view.id;
            return (
              <button className={`context-row ${isSelected ? "current" : ""}`} key={view.id} onClick={() => onSelect(view.id)} type="button">
                <div className="row-title">
                  <span>{view.title}</span>
                  {isSelected ? <span className="badge">selected</span> : null}
                </div>
                <div className="row-meta">{view.description}</div>
              </button>
            );
          })}
        </div>
      </CardBody>
    </Card>
  );
}

function CreateContextForm({ onCreate }: { onCreate(req: CreateContextRequest): Promise<void> }) {
  const [form, setForm] = useState<CreateContextRequest>({
    endpoint: "",
    name: "",
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
            <label htmlFor="context-description">Description</label>
            <TextInput id="context-description" onChange={(e) => update("description", e.target.value)} value={form.description ?? ""} />
          </div>
          <Button type="submit">Create Context</Button>
        </form>
      </CardBody>
    </Card>
  );
}

function LoadingPage({ error, title }: { error: string; title: string }) {
  return (
    <div className="loading-shell">
      <div className="loading-panel">
        <div className="brand loading-brand">
          <div className="brand-mark" />
          <span>GizClaw Desktop</span>
        </div>
        <div className="loading-spinner" />
        <div>
          <div className="loading-title">{title}</div>
          <div className="loading-meta">Preparing runtime and WebRTC session...</div>
        </div>
        {error ? <div className="error">{error}</div> : null}
      </div>
    </div>
  );
}

function defaultViews(): DesktopView[] {
  return [
    { description: "Manage GizClaw server resources.", id: "admin", title: "Admin" },
    { description: "Use workspaces, chat history, social, and firmware flows.", id: "play", title: "Play" },
  ];
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
