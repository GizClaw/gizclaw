import { RefreshCw } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Button } from "../../components/Button";
import { Card, CardBody, CardHeader } from "../../components/Card";
import {
  type AdminSection,
  type AdminDataClient,
  type AdminRow,
  connectAdminSession,
  getInjectedAdminDataClient,
} from "../../lib/gizclaw/admin";
import type { RuntimeContext } from "../../lib/runtime/types";

export function AdminHome({ runtime }: { runtime: RuntimeContext }) {
  const [sections, setSections] = useState<AdminSection[]>([]);
  const [activeKey, setActiveKey] = useState<string>("peers");
  const [selectedID, setSelectedID] = useState<string>("");
  const [client, setClient] = useState<AdminDataClient | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const injectedClient = useMemo(() => getInjectedAdminDataClient(), []);

  useEffect(() => {
    let cancelled = false;
    let close: (() => void) | undefined;
    setClient(undefined);
    setSections([]);
    setError("");
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
    connectAdminSession(runtime)
      .then((session) => {
        if (cancelled) {
          session.close();
          return;
        }
        close = () => session.close();
        setClient(session);
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
      close?.();
    };
  }, [injectedClient, runtime]);

  useEffect(() => {
    if (client) {
      void load();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client]);

  async function load() {
    setError("");
    if (!runtime.context) {
      setSections([]);
      return;
    }
    if (!client) {
      setSections([]);
      return;
    }
    setLoading(true);
    try {
      const next = await client.listSections();
      setSections(next);
      if (next.length > 0 && !next.some((section) => section.key === activeKey)) {
        setActiveKey(next[0].key);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }

  const active = sections.find((section) => section.key === activeKey) ?? sections[0];
  const selected = active?.rows.find((row) => row.id === selectedID) ?? active?.rows[0];

  return (
    <div className="admin-view">
      <Card>
        <CardHeader
          action={
            <Button onClick={() => void load()} type="button" variant="secondary">
              <RefreshCw size={15} /> Refresh
            </Button>
          }
          title="Admin API"
        />
        <CardBody>
          <div className="details">
            <div className="detail-row">
              <div className="detail-key">Transport</div>
              <div>WebRTC Admin API</div>
            </div>
            <div className="detail-row">
              <div className="detail-key">Server</div>
              <div className="mono">{runtime.context?.endpoint ?? "No context selected"}</div>
            </div>
            <div className="detail-row">
              <div className="detail-key">State</div>
              <div>{loading ? "Loading" : client ? "Connected" : "Waiting for WebRTC session"}</div>
            </div>
          </div>
          {error ? <div className="error compact">{error}</div> : null}
        </CardBody>
      </Card>

      {sections.length > 0 ? (
        <>
          <ResourceOverview sections={sections} activeKey={activeKey} onSelect={setActiveKey} />
          {active ? (
            <div className="grid-two admin-detail-grid">
              <ResourceTable section={active} selectedID={selected?.id ?? ""} onSelect={setSelectedID} />
              {selected ? <ResourceDetail row={selected} section={active} /> : null}
            </div>
          ) : null}
        </>
      ) : (
        <Card>
          <CardHeader title="Resources" />
          <CardBody>
            <div className="empty">
              {runtime.context ? "Connect the Admin WebRTC session to load resources." : "Select a context to load Admin resources."}
            </div>
          </CardBody>
        </Card>
      )}
    </div>
  );
}

function ResourceOverview({
  activeKey,
  onSelect,
  sections,
}: {
  activeKey: string;
  onSelect(key: string): void;
  sections: AdminSection[];
}) {
  return (
    <div className="resource-grid">
      {sections.map((section) => (
        <button
          className={`resource-tile ${section.key === activeKey ? "active" : ""}`}
          key={section.key}
          onClick={() => onSelect(section.key)}
          type="button"
        >
          <div className="resource-count">{section.rows.length}</div>
          <div className="resource-title">{section.title}</div>
          <div className="resource-description">{section.description}</div>
        </button>
      ))}
    </div>
  );
}

function ResourceTable({
  onSelect,
  section,
  selectedID,
}: {
  onSelect(id: string): void;
  section: AdminSection;
  selectedID: string;
}) {
  return (
    <Card>
      <CardHeader title={section.title} />
      <CardBody>
        {section.rows.length === 0 ? (
          <div className="empty">No {section.title.toLowerCase()} found.</div>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Updated</th>
                </tr>
              </thead>
              <tbody>
                {section.rows.map((row) => (
                  <tr className={row.id === selectedID ? "selected" : ""} key={row.id}>
                    <td>
                      <button className="table-link mono" onClick={() => onSelect(row.id)} type="button">
                        {row.id}
                      </button>
                      {row.subtitle ? <div className="table-subtitle">{row.subtitle}</div> : null}
                    </td>
                    <td>{row.title}</td>
                    <td>{row.status ?? ""}</td>
                    <td>{row.updated_at ?? ""}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardBody>
    </Card>
  );
}

function ResourceDetail({ row, section }: { row: AdminRow; section: AdminSection }) {
  return (
    <Card>
      <CardHeader title={`${section.title} Detail`} />
      <CardBody>
        <div className="details">
          <div className="detail-row">
            <div className="detail-key">ID</div>
            <div className="mono">{row.id}</div>
          </div>
          <div className="detail-row">
            <div className="detail-key">Name</div>
            <div>{row.title}</div>
          </div>
          <div className="detail-row">
            <div className="detail-key">Status</div>
            <div>{row.status ?? ""}</div>
          </div>
          <div className="detail-row">
            <div className="detail-key">CLI</div>
            <div className="mono">gizclaw admin --context &lt;admin-cli-context&gt; show {resourceKind(section.key)} '{row.id}'</div>
          </div>
        </div>
        <pre className="json-panel">{JSON.stringify(row.raw ?? row, null, 2)}</pre>
      </CardBody>
    </Card>
  );
}

function resourceKind(sectionKey: string): string {
  return sectionKey
    .split("-")
    .map((part) => `${part.slice(0, 1).toUpperCase()}${part.slice(1)}`)
    .join("");
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
