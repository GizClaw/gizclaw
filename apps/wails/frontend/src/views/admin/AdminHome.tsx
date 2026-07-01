import type { RuntimeContext } from "../../lib/runtime/types";
import { Card, CardBody, CardHeader } from "../../components/Card";

export function AdminHome({ runtime }: { runtime: RuntimeContext }) {
  return (
    <Card>
      <CardHeader title="Admin" />
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
        </div>
      </CardBody>
    </Card>
  );
}
