import { useState, type FormEvent } from "react";
import { Plus, ScrollText, Search, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { PageHeading } from "@/components/app-shell";
import type { ConsoleConfig } from "@/lib/config";
import {
  findPeers,
  normalizePublicKey,
  peerId,
  peerLabel,
  type WatchedPeer,
} from "@/lib/peers";
import { nodeErrorMessage } from "@/lib/api";
import type { PeerState } from "@/hooks/use-peers";
import { bytes, host, timestamp } from "@/lib/format";

export function PeersPage({
  config,
  peers,
  states,
  onAdd,
  onRemove,
  storageError,
}: {
  config: ConsoleConfig;
  peers: WatchedPeer[];
  states: Record<string, PeerState>;
  onAdd: (peer: WatchedPeer) => void;
  onRemove: (peer: WatchedPeer) => void;
  storageError: string;
}) {
  // The console is served from the access point, so device APIs live on this
  // page's own origin unless the configuration names another one.
  const endpoint = config.deviceEndpoint ?? window.location.origin;
  const add = (publicKey: string, label: string) =>
    onAdd({
      publicKey: normalizePublicKey(publicKey),
      label: label.trim(),
      endpoint,
      addedAt: Date.now(),
    });
  return (
    <>
      <PageHeading
        eyebrow="DEVICE WATCH LIST"
        title="设备监控"
        description={`设备数据来自本站接入点 ${host(endpoint)}。关注列表只存在这台浏览器里，各人维护各自的设备，可随配置一起导出。`}
      />
      {storageError && (
        <Alert>
          <AlertTitle>关注列表保存失败</AlertTitle>
          <AlertDescription>{storageError}</AlertDescription>
        </Alert>
      )}
      <AddPeerCard onAdd={add} />
      <SearchPeerCard endpoint={endpoint} onAdd={add} />
      <Card>
        <CardHeader>
          <CardTitle>关注的设备</CardTitle>
          <CardDescription>
            每 5 秒读取一次设备状态；权限由接入点所属的 Server 再次校验。
          </CardDescription>
        </CardHeader>
        <CardContent className="px-0">
          {peers.length === 0 ? (
            <p className="px-5 py-10 text-center text-xs text-muted-foreground">
              还没有关注任何设备。
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-5">设备</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>上行 / 下行</TableHead>
                  <TableHead>调试权限</TableHead>
                  <TableHead>最后活跃</TableHead>
                  <TableHead className="pr-5 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {peers.map((peer) => {
                  const state = states[peerId(peer)];
                  const runtime = state?.snapshot?.runtime;
                  return (
                    <TableRow key={peerId(peer)}>
                      <TableCell className="pl-5">
                        <a
                          className="font-medium hover:text-primary"
                          href={`#/peer/${encodeURIComponent(peer.publicKey)}`}
                        >
                          {peerLabel(peer, state?.snapshot)}
                        </a>
                        <div className="mt-1 max-w-80 truncate font-mono text-[10px] text-muted-foreground">
                          {peer.publicKey}
                        </div>
                      </TableCell>
                      <TableCell>
                        {state?.status === "error" ? (
                          <>
                            <Badge variant="destructive">无法读取</Badge>
                            <div className="mt-1 max-w-64 truncate text-[10px] text-destructive">
                              {state.error}
                            </div>
                          </>
                        ) : runtime ? (
                          <Badge
                            variant={runtime.online ? "success" : "outline"}
                          >
                            {runtime.online ? "在线" : "离线"}
                          </Badge>
                        ) : (
                          <Badge variant="outline">读取中</Badge>
                        )}
                      </TableCell>
                      <TableCell className="tabular-nums">
                        {runtime?.rx_bytes !== undefined &&
                        runtime.tx_bytes !== undefined
                          ? `${bytes(runtime.rx_bytes)} · ${bytes(runtime.tx_bytes)}`
                          : "—"}
                      </TableCell>
                      <TableCell>{runtime?.debug_mode ?? "—"}</TableCell>
                      <TableCell>{timestamp(runtime?.last_seen_at)}</TableCell>
                      <TableCell className="pr-5">
                        <div className="flex justify-end gap-2">
                          <Button variant="outline" size="sm" asChild>
                            <a
                              href={`#/peer/${encodeURIComponent(peer.publicKey)}`}
                            >
                              详情
                            </a>
                          </Button>
                          <Button variant="outline" size="sm" asChild>
                            <a
                              href={`#/logs?q=${encodeURIComponent(`peer_public_key:${peer.publicKey}`)}`}
                            >
                              <ScrollText size={13} /> 节点日志
                            </a>
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`移除 ${peerLabel(peer, state?.snapshot)}`}
                            onClick={() => onRemove(peer)}
                          >
                            <Trash2 size={13} /> 移除
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
      {peers.map((peer) => {
        const state = states[peerId(peer)];
        if (!state?.snapshot) return null;
        return (
          <Card key={`${peerId(peer)}-detail`}>
            <CardHeader>
              <CardTitle>
                {peerLabel(peer, state.snapshot)} · 原始记录
              </CardTitle>
              <CardDescription>
                设备信息、运行时状态与 Server 记录的状态字段，空值保持为空。
              </CardDescription>
            </CardHeader>
            <CardContent>
              <details>
                <summary className="cursor-pointer text-xs text-muted-foreground">
                  展开 JSON
                </summary>
                <pre className="mt-3 max-h-96 overflow-auto text-[11px] leading-relaxed text-muted-foreground">
                  {JSON.stringify(state.snapshot, null, 2)}
                </pre>
              </details>
            </CardContent>
          </Card>
        );
      })}
    </>
  );
}

/** Adding is always by public key; the search card below produces those keys. */
function AddPeerCard({
  onAdd,
}: {
  onAdd: (publicKey: string, label: string) => void;
}) {
  const [publicKey, setPublicKey] = useState("");
  const [label, setLabel] = useState("");
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (normalizePublicKey(publicKey) === "") return;
    onAdd(publicKey, label);
    setPublicKey("");
    setLabel("");
  };
  return (
    <Card>
      <CardHeader>
        <CardTitle>添加设备</CardTitle>
        <CardDescription>
          用设备公钥关注一台设备；不知道公钥时，用下面的搜索按 SN 或 IMEI
          找到它。
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form className="flex flex-wrap items-center gap-2" onSubmit={submit}>
          <Input
            aria-label="设备公钥"
            value={publicKey}
            spellCheck={false}
            onChange={(event) => setPublicKey(event.target.value)}
            placeholder="gizclaw_pk_…"
            className="w-96 font-mono"
          />
          <Input
            aria-label="备注名称"
            value={label}
            onChange={(event) => setLabel(event.target.value)}
            placeholder="备注名称（可选）"
            className="w-44"
          />
          <Button type="submit" disabled={normalizePublicKey(publicKey) === ""}>
            <Plus size={14} /> 添加
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

/** SN and IMEI are a lookup: they resolve to public keys, nothing more. */
function SearchPeerCard({
  endpoint,
  onAdd,
}: {
  endpoint: string;
  onAdd: (publicKey: string, label: string) => void;
}) {
  const [kind, setKind] = useState<"sn" | "imei">("sn");
  const [value, setValue] = useState("");
  const [serial, setSerial] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [results, setResults] = useState<string[] | undefined>();

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    setResults(undefined);
    const controller = new AbortController();
    findPeers(endpoint, kind, value, serial, controller.signal)
      .then(setResults)
      .catch((failure: unknown) => setError(nodeErrorMessage(failure)))
      .finally(() => setBusy(false));
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>按 SN / IMEI 搜索</CardTitle>
        <CardDescription>
          查询接入点的公开标识索引，返回匹配的设备公钥；标识重复时会返回多个。
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <form className="flex flex-wrap items-center gap-2" onSubmit={submit}>
          <Select
            aria-label="搜索方式"
            value={kind}
            onChange={(event) => setKind(event.target.value as "sn" | "imei")}
          >
            <option value="sn">SN</option>
            <option value="imei">IMEI</option>
          </Select>
          <Input
            aria-label="搜索标识"
            value={value}
            onChange={(event) => setValue(event.target.value)}
            placeholder={kind === "sn" ? "设备 SN" : "IMEI TAC"}
            className="w-64"
          />
          {kind === "imei" && (
            <Input
              aria-label="IMEI 序列号"
              value={serial}
              onChange={(event) => setSerial(event.target.value)}
              placeholder="IMEI 序列号"
              className="w-40"
            />
          )}
          <Button
            type="submit"
            variant="outline"
            disabled={busy || value.trim() === ""}
          >
            <Search size={14} /> {busy ? "搜索中" : "搜索"}
          </Button>
        </form>
        {error && <p className="text-[11px] text-destructive">{error}</p>}
        {results !== undefined &&
          (results.length === 0 ? (
            <p className="text-[11px] text-muted-foreground">
              没有匹配的设备。
            </p>
          ) : (
            <div className="flex flex-col gap-2">
              <p className="text-[11px] text-muted-foreground">
                匹配到 {results.length} 个公钥：
              </p>
              {results.map((key) => (
                <div
                  key={key}
                  className="flex items-center gap-3 rounded-md border border-border px-3 py-2"
                >
                  <code className="min-w-0 flex-1 truncate font-mono text-[11px]">
                    {key}
                  </code>
                  <Button size="sm" onClick={() => onAdd(key, "")}>
                    <Plus size={13} /> 添加
                  </Button>
                </div>
              ))}
            </div>
          ))}
      </CardContent>
    </Card>
  );
}
