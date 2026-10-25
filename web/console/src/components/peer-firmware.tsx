import { useEffect, useRef, useState } from "react";
import { RefreshCw, Upload } from "lucide-react";
import type {
  DeviceFirmware,
  FirmwareChannelName,
} from "@gizclaw/gizclaw/peerhttp";
import { GizClawControlError } from "@gizclaw/gizclaw-control";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { PeerState } from "@/hooks/use-peers";
import { nodeErrorMessage } from "@/lib/api";
import { bytes, timestamp } from "@/lib/format";
import { peerAuth, type PeerReadAuth } from "@/lib/peer-auth";
import {
  firmwareChannels,
  loadPeerFirmware,
  peerId,
  peerOtaStatus,
  updatePeerFirmware,
  type WatchedPeer,
} from "@/lib/peers";

type Props = { peer: WatchedPeer; state: PeerState | undefined };

export function PeerFirmware(props: Props) {
  return <FirmwareControl key={peerId(props.peer)} {...props} />;
}

function FirmwareControl({ peer, state }: Props) {
  const auth = peerAuth(peer);
  const [loaded, setLoaded] = useState<{
    firmware: DeviceFirmware;
    auth: PeerReadAuth | undefined;
  }>();
  const [channel, setChannel] = useState<FirmwareChannelName>();
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [reload, setReload] = useState(0);
  const [confirming, setConfirming] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [accepted, setAccepted] = useState("");
  const [updateError, setUpdateError] = useState("");
  const pending = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setLoaded(undefined);
    setChannel(undefined);
    setLoadError("");
    setConfirming(false);
    setUpdating(false);
    setAccepted("");
    setUpdateError("");
    loadPeerFirmware(
      peer.endpoint,
      peer.publicKey,
      AbortSignal.any([controller.signal, AbortSignal.timeout(30_000)]),
      auth,
    )
      .then((firmware) => {
        if (controller.signal.aborted) return;
        setLoaded({ firmware, auth });
        setChannel(
          firmwareChannels.find((name) => firmware.slots[name].package),
        );
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setLoadError(
          error instanceof GizClawControlError &&
            error.code === "FIRMWARE_NOT_FOUND"
            ? "该设备未绑定固件配置，或绑定的配置已删除。"
            : nodeErrorMessage(error),
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      controller.abort();
      pending.current?.abort();
      pending.current = null;
    };
  }, [peer.endpoint, peer.publicKey, auth, reload]);

  const firmware = loaded?.auth === auth ? loaded?.firmware : undefined;
  const slot = channel ? firmware?.slots[channel] : undefined;
  const pkg = slot?.package;
  const online =
    state?.status === "online" && state.snapshot?.runtime.online === true;
  const blocked = !online
    ? state?.status === "error"
      ? "设备状态读取失败，恢复读取后可推送。"
      : "设备在线后才可推送固件。"
    : !auth && state?.snapshot?.runtime.debug_mode !== "fullcontrol"
      ? "推送需要有控制权限的 admin 身份、单设备签名凭证，或设备开启 fullcontrol 调试模式。"
      : "";
  const canPush = !!pkg && !loading && !updating && blocked === "";
  const ota = peerOtaStatus(state?.snapshot?.status ?? {});

  async function push() {
    if (!canPush || !pkg || !channel || pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setConfirming(false);
    setUpdating(true);
    setAccepted("");
    setUpdateError("");
    try {
      await updatePeerFirmware(
        peer.endpoint,
        peer.publicKey,
        { channel, sha256: pkg.sha256 },
        AbortSignal.any([controller.signal, AbortSignal.timeout(30_000)]),
        auth,
      );
      if (!controller.signal.aborted)
        setAccepted(
          `设备已接受 ${channel} · ${pkg.version ?? "版本未标注"} 的升级请求。请查看后续 OTA 上报，确认升级结果。`,
        );
    } catch (error) {
      if (!controller.signal.aborted)
        setUpdateError(
          `${nodeErrorMessage(error)}。请先查看设备状态和 OTA 上报，确认结果后再决定是否重试。`,
        );
    } finally {
      if (!controller.signal.aborted) {
        pending.current = null;
        setUpdating(false);
      }
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>固件推送</CardTitle>
          <CardDescription>
            选择该设备已绑定的固件 channel，向当前设备发送升级请求。
          </CardDescription>
          <CardAction>
            <Button
              variant="outline"
              size="sm"
              disabled={loading || updating}
              onClick={() => setReload((value) => value + 1)}
            >
              <RefreshCw size={13} /> {loading ? "读取中" : "刷新固件"}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {loading && (
            <p className="text-xs text-muted-foreground">
              正在读取设备绑定的固件…
            </p>
          )}
          {loadError && (
            <Alert>
              <AlertTitle>无法读取固件</AlertTitle>
              <AlertDescription>{loadError}</AlertDescription>
            </Alert>
          )}
          {firmware && (
            <>
              {firmware.description && (
                <p className="text-xs text-muted-foreground">
                  {firmware.description}
                </p>
              )}
              <div className="flex flex-wrap items-end gap-3">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="firmware-channel">固件 channel</Label>
                  <Select
                    id="firmware-channel"
                    value={channel ?? ""}
                    disabled={updating || !pkg}
                    onChange={(event) => {
                      const selected = firmwareChannels.find(
                        (name) => name === event.target.value,
                      );
                      setChannel(selected);
                      setConfirming(false);
                    }}
                  >
                    {!channel && <option value="">没有可推送的固件</option>}
                    {firmwareChannels.map((name) => (
                      <option
                        key={name}
                        value={name}
                        disabled={!firmware.slots[name].package}
                      >
                        {name} ·{" "}
                        {firmware.slots[name].package?.version ??
                          (firmware.slots[name].package
                            ? "版本未标注"
                            : "未配置固件")}
                      </option>
                    ))}
                  </Select>
                </div>
                <Button disabled={!canPush} onClick={() => setConfirming(true)}>
                  <Upload size={14} />
                  {updating ? "正在推送…" : "推送固件"}
                </Button>
              </div>
              {pkg ? (
                <div className="rounded-md border border-border p-3 text-xs">
                  <p>
                    目标版本：{pkg.version ?? "版本未标注"} · {bytes(pkg.size)}
                  </p>
                  {slot?.description && (
                    <p className="mt-2 text-muted-foreground">
                      {slot.description}
                    </p>
                  )}
                  <p className="mt-2 break-all font-mono text-[11px] text-muted-foreground">
                    SHA-256 {pkg.sha256}
                  </p>
                </div>
              ) : (
                <p className="text-xs text-muted-foreground">
                  该设备绑定的固件配置中没有可推送的包。
                </p>
              )}
              {blocked && (
                <p className="text-xs text-muted-foreground">{blocked}</p>
              )}
            </>
          )}
          {accepted && (
            <p role="status" className="text-xs text-foreground">
              {accepted}
            </p>
          )}
          {updateError && (
            <Alert>
              <AlertTitle>推送未确认</AlertTitle>
              <AlertDescription>{updateError}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>OTA 状态</CardTitle>
          <CardDescription>
            最近一次设备上报的升级状态；随设备状态轮询更新。
          </CardDescription>
          {state?.status === "error" && (
            <CardAction>
              <Badge variant="outline">上次快照</Badge>
            </CardAction>
          )}
        </CardHeader>
        <CardContent className="flex flex-col gap-2 text-xs">
          {ota ? (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <Badge
                  variant={
                    ota.state === "failed"
                      ? "destructive"
                      : ota.state === "succeeded"
                        ? "success"
                        : "outline"
                  }
                >
                  {otaState(ota.state)}
                </Badge>
                {ota.target_version && (
                  <span>目标版本 {ota.target_version}</span>
                )}
                <span className="text-muted-foreground">
                  上报于 {timestamp(ota.observed_at)}
                </span>
              </div>
              <p className="break-all font-mono text-[11px] text-muted-foreground">
                升级 ID：{ota.update_id}
              </p>
              {ota.download_percent !== undefined && (
                <div className="flex items-center gap-3">
                  <progress
                    aria-label="固件下载进度"
                    max={100}
                    value={ota.download_percent}
                    className="h-2 w-full max-w-sm accent-primary"
                  />
                  <span>{ota.download_percent}%</span>
                </div>
              )}
              {(ota.error_code || ota.error_message) && (
                <p className="break-words text-destructive">
                  {[ota.error_code, ota.error_message]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
              )}
            </>
          ) : (
            <p className="text-muted-foreground">
              尚无 OTA 上报。请求被接受后，等待设备上报升级进度与结果。
            </p>
          )}
        </CardContent>
      </Card>
      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认推送固件</DialogTitle>
            <DialogDescription>
              设备将下载并安装所选固件，升级期间可能断线或重启。
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2 text-xs">
            <p>设备：{peer.label || peer.publicKey}</p>
            <p className="break-all font-mono text-[11px] text-muted-foreground">
              {peer.publicKey}
            </p>
            <p>
              channel：{channel} · 目标版本：{pkg?.version ?? "版本未标注"}
            </p>
            <p className="break-all font-mono text-[11px] text-muted-foreground">
              SHA-256 {pkg?.sha256}
            </p>
            {blocked && <p className="text-destructive">{blocked}</p>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(false)}>
              取消
            </Button>
            <Button disabled={!canPush} onClick={() => void push()}>
              确认推送
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function otaState(state: string): string {
  switch (state) {
    case "started":
      return "已开始";
    case "downloading":
      return "下载中";
    case "succeeded":
      return "升级成功";
    case "failed":
      return "升级失败";
    default:
      return state;
  }
}
