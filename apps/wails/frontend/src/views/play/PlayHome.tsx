import type { RuntimeContext } from "../../lib/runtime/types";
import { Card, CardBody, CardHeader } from "../../components/Card";

export function PlayHome({ runtime }: { runtime: RuntimeContext }) {
  return (
    <Card>
      <CardHeader title="Play" />
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
        </div>
      </CardBody>
    </Card>
  );
}
