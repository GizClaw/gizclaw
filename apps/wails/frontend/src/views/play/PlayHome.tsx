import { Clock3, History, MessageCircle, PackageCheck, RefreshCw, RotateCw, Search, Users } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Button } from "../../components/Button";
import { Card, CardBody, CardHeader } from "../../components/Card";
import { TextInput } from "../../components/TextInput";
import {
  type PlayDataClient,
  type PlayHistoryRow,
  type PlayMemoryRecall,
  type PlayResourceRow,
  type PlaySession,
  type PlaySnapshot,
  connectPlaySession,
  getInjectedPlayDataClient,
} from "../../lib/gizclaw/play";
import type { RuntimeContext } from "../../lib/runtime/types";

const emptySnapshot: PlaySnapshot = {
  contacts: [],
  credentials: [],
  firmwares: [],
  friendGroups: [],
  friends: [],
  history: [],
  models: [],
  pets: [],
  rewards: [],
  warnings: [],
  walletTransactions: [],
  workflows: [],
  workspaces: [],
};

export function PlayHome({ runtime }: { runtime: RuntimeContext }) {
  const [client, setClient] = useState<PlayDataClient | undefined>();
  const [snapshot, setSnapshot] = useState<PlaySnapshot>(emptySnapshot);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [lastAction, setLastAction] = useState("");
  const [workspaceName, setWorkspaceName] = useState("");
  const [recallQuery, setRecallQuery] = useState("");
  const [recall, setRecall] = useState<PlayMemoryRecall | undefined>();
  const injectedClient = useMemo(() => getInjectedPlayDataClient(), []);

  useEffect(() => {
    let cancelled = false;
    let session: PlaySession | undefined;
    setClient(undefined);
    setSnapshot(emptySnapshot);
    setError("");
    setLastAction("");
    if (!runtime.context) {
      return () => {
        cancelled = true;
      };
    }
    if (injectedClient) {
      setClient(injectedClient);
      return () => {
        cancelled = true;
      };
    }
    setLoading(true);
    connectPlaySession(runtime)
      .then((next) => {
        if (cancelled) {
          next.close();
          return;
        }
        session = next;
        setClient(next);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(errorMessage(err));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
      session?.close();
    };
  }, [injectedClient, runtime]);

  useEffect(() => {
    if (client) {
      void refresh();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client]);

  async function refresh() {
    if (!runtime.context || !client) {
      setSnapshot(emptySnapshot);
      return;
    }
    setError("");
    setLoading(true);
    try {
      const next = await client.loadSnapshot();
      setSnapshot(next);
      const activeWorkspace = next.runWorkspace?.workspace_name ?? next.runWorkspace?.name ?? "";
      setWorkspaceName(activeWorkspace);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }

  async function runAction(action: () => Promise<unknown>, message: string) {
    if (!client) {
      return;
    }
    setError("");
    setLastAction("");
    setLoading(true);
    try {
      await action();
      setLastAction(message);
      await refresh();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }

  function submitWorkspace(event: FormEvent) {
    event.preventDefault();
    const name = workspaceName.trim();
    if (name !== "") {
      void runAction(() => client?.setWorkspace(name) ?? Promise.resolve(), `Workspace set to ${name}.`);
    }
  }

  function submitRecall(event: FormEvent) {
    event.preventDefault();
    const query = recallQuery.trim();
    if (!client || query === "") {
      return;
    }
    setError("");
    setLoading(true);
    client
      .recallMemory(query)
      .then(setRecall)
      .catch((err: unknown) => setError(errorMessage(err)))
      .finally(() => setLoading(false));
  }

  const runWorkspace = snapshot.runWorkspace;

  return (
    <div className="play-view">
      <Card>
        <CardHeader
          action={
            <Button onClick={() => void refresh()} type="button" variant="secondary">
              <RefreshCw size={15} /> Refresh
            </Button>
          }
          title="Play RPC"
        />
        <CardBody>
          <div className="details">
            <div className="detail-row">
              <div className="detail-key">Transport</div>
              <div>WebRTC RPC</div>
            </div>
            <div className="detail-row">
              <div className="detail-key">Context</div>
              <div className="mono">{runtime.context?.name ?? "No context selected"}</div>
            </div>
            <div className="detail-row">
              <div className="detail-key">State</div>
              <div>{loading ? "Loading" : client ? "Connected" : "Waiting for WebRTC session"}</div>
            </div>
          </div>
          {error ? <div className="error compact">{error}</div> : null}
          {snapshot.warnings.length > 0 ? (
            <div className="warning-list">
              {snapshot.warnings.map((warning) => (
                <div className="warning compact" key={warning}>
                  {warning}
                </div>
              ))}
            </div>
          ) : null}
          {lastAction ? <div className="success compact">{lastAction}</div> : null}
        </CardBody>
      </Card>

      <div className="grid-two">
        <Card>
          <CardHeader
            action={
              <Button
                onClick={() => void runAction(() => client?.reloadWorkspace() ?? Promise.resolve(), "Workspace reloaded.")}
                type="button"
                variant="secondary"
              >
                <RotateCw size={15} /> Reload
              </Button>
            }
            title="Workspace"
          />
          <CardBody>
            <form className="inline-form" onSubmit={submitWorkspace}>
              <TextInput
                aria-label="Workspace name"
                onChange={(event) => setWorkspaceName(event.target.value)}
                placeholder="workspace-name"
                value={workspaceName}
              />
              <Button type="submit">Set</Button>
            </form>
            <div className="details compact-details">
              <div className="detail-row">
                <div className="detail-key">Active</div>
                <div className="mono">{runWorkspace?.workspace_name ?? runWorkspace?.name ?? "None"}</div>
              </div>
              <div className="detail-row">
                <div className="detail-key">Mode</div>
                <div>{runWorkspace?.mode ?? "Default"}</div>
              </div>
              <div className="detail-row">
                <div className="detail-key">Memory</div>
                <div>{snapshot.memoryStats?.total ?? "Unknown"}</div>
              </div>
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Memory Recall" action={<Search size={17} />} />
          <CardBody>
            <form className="inline-form" onSubmit={submitRecall}>
              <TextInput
                aria-label="Memory recall query"
                onChange={(event) => setRecallQuery(event.target.value)}
                placeholder="Search memory"
                value={recallQuery}
              />
              <Button type="submit">Recall</Button>
            </form>
            <ResourceList empty="No recall hits." rows={recall?.hits ?? []} />
          </CardBody>
        </Card>
      </div>

      <div className="grid-two">
        <Card>
          <CardHeader title="Workspace History" action={<History size={17} />} />
          <CardBody>
            <HistoryList history={snapshot.history} onPlay={(id) => void runAction(() => client?.playHistory(id) ?? Promise.resolve(), `History ${id} replay requested.`)} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Social" action={<Users size={17} />} />
          <CardBody>
            <div className="metric-grid">
              <Metric label="Contacts" value={snapshot.contacts.length} />
              <Metric label="Friends" value={snapshot.friends.length} />
              <Metric label="Groups" value={snapshot.friendGroups.length} />
            </div>
            <ResourceList empty="No friends found." rows={[...snapshot.friends, ...snapshot.friendGroups]} />
          </CardBody>
        </Card>
      </div>

      <div className="grid-two">
        <Card>
          <CardHeader title="Resource Catalog" action={<PackageCheck size={17} />} />
          <CardBody>
            <div className="metric-grid">
              <Metric label="Workspaces" value={snapshot.workspaces.length} />
              <Metric label="Workflows" value={snapshot.workflows.length} />
              <Metric label="Models" value={snapshot.models.length} />
            </div>
            <ResourceList
              empty="No catalog resources found."
              rows={[
                ...snapshot.workspaces,
                ...snapshot.workflows,
                ...snapshot.models,
                ...snapshot.credentials,
              ]}
            />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Gameplay" action={<PackageCheck size={17} />} />
          <CardBody>
            <div className="metric-grid">
              <Metric label="Pets" value={snapshot.pets.length} />
              <Metric label="Rewards" value={snapshot.rewards.length} />
              <Metric label="Wallet Tx" value={snapshot.walletTransactions.length} />
            </div>
            <ResourceList
              empty="No gameplay resources found."
              rows={[
                ...(snapshot.wallet ? [snapshot.wallet] : []),
                ...snapshot.pets,
                ...snapshot.rewards,
                ...snapshot.walletTransactions,
              ]}
            />
          </CardBody>
        </Card>
      </div>

      <Card>
        <CardHeader title="Firmwares" action={<PackageCheck size={17} />} />
        <CardBody>
          <ResourceList empty="No firmware resources found." rows={snapshot.firmwares} />
        </CardBody>
      </Card>
    </div>
  );
}

function HistoryList({ history, onPlay }: { history: PlayHistoryRow[]; onPlay(id: string): void }) {
  if (history.length === 0) {
    return <div className="empty">No history entries found.</div>;
  }
  return (
    <div className="history-list">
      {history.map((entry) => (
        <div className="history-row" key={entry.id}>
          <div className="history-main">
            <div className="row-title">
              <MessageCircle size={15} />
              <span>{entry.name ?? entry.type ?? entry.id}</span>
            </div>
            <div className="row-meta">{entry.text ?? entry.id}</div>
            {entry.updated_at ? (
              <div className="row-meta">
                <Clock3 size={13} /> {entry.updated_at}
              </div>
            ) : null}
          </div>
          <Button onClick={() => onPlay(entry.id)} type="button" variant="secondary">
            Play
          </Button>
        </div>
      ))}
    </div>
  );
}

function ResourceList({ empty, rows }: { empty: string; rows: PlayResourceRow[] }) {
  if (rows.length === 0) {
    return <div className="empty">{empty}</div>;
  }
  return (
    <div className="compact-list">
      {rows.map((row) => (
        <div className="compact-row" key={row.id}>
          <div>
            <div className="row-title">{row.title}</div>
            <div className="row-meta mono">{row.id}</div>
            {row.subtitle ? <div className="row-meta">{row.subtitle}</div> : null}
          </div>
          {row.updated_at ? <div className="row-meta">{row.updated_at}</div> : null}
        </div>
      ))}
    </div>
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="metric">
      <div className="metric-value">{value}</div>
      <div className="metric-label">{label}</div>
    </div>
  );
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
