// Keep bounded transport metadata for failed native RPC scenarios. Never
// retain message contents: device requests can contain management passwords.
import {
  GIZCLAW_SERVICE_PEER_RPC,
  giznetServiceDataChannelLabel,
} from "@gizclaw/gizclaw";

type ChannelEvent = {
  event: string;
  channel_id: number | null;
  ready_state: RTCDataChannelState;
  bytes?: number;
};

export function tracePeerRPCChannels(pc: RTCPeerConnection): ChannelEvent[] {
  const events: ChannelEvent[] = [];
  pc.addEventListener("datachannel", ({ channel }) => {
    if (
      channel.label !== giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC)
    )
      return;
    const record = (event: string, bytes?: number): void => {
      events.push({
        event,
        channel_id: channel.id,
        ready_state: channel.readyState,
        ...(bytes == null ? {} : { bytes }),
      });
      if (events.length > 64) events.shift();
    };
    record("channel");
    for (const event of ["open", "close", "error"]) {
      channel.addEventListener(event, () => record(event));
    }
    channel.addEventListener("message", ({ data }: MessageEvent<unknown>) => {
      const bytes =
        data instanceof ArrayBuffer || ArrayBuffer.isView(data)
          ? data.byteLength
          : data instanceof Blob
            ? data.size
            : undefined;
      record("message", bytes);
    });
  });
  return events;
}
