"""Require correlated evidence of Server A -> other Edge -> Server B -> ACK."""

import json
import re
import sys
from pathlib import Path


def main():
    tests = Path(sys.argv[1]).read_text(encoding="utf-8")
    services = Path(sys.argv[2]).read_text(encoding="utf-8").splitlines()
    topology = re.search(
        r"TOPOLOGY source=(\S+) server=(\S+) ingress_edge=(\S+); "
        r"local=(\S+) server=(\S+) direct; "
        r"target=(\S+) server=(\S+) ingress_edge=(\S+) selected_relay_edge=(\S+)",
        tests,
    )
    if topology is None:
        raise SystemExit("social ping topology evidence is missing")
    source, source_server, source_edge, local, local_server, target, target_server, ingress, relay = topology.groups()
    if ingress == relay or source_server == target_server or source_edge != relay:
        raise SystemExit("social ping topology does not exercise a different relay Edge")
    for line in services:
        if ('msg="social ping relayed"' not in line
                or f"target_peer_public_key={target} " not in line
                or f"edge_public_key={relay} " not in line
                or "acknowledged=true" not in line):
            continue
        trace = re.search(r"request_id=(\S+)", line)
        if trace is None:
            continue
        request_id = trace.group(1)
        routed = next((item for item in services
            if 'msg="social ping routed"' in item
            and f"request_id={request_id} " in item
            and f"target_peer_public_key={target} " in item
            and f"source_server_public_key={source_server} " in item
            and f"target_server_public_key={target_server} " in item
            and "acknowledged=true" in item), None)
        acknowledged = next((item for item in services
            if "server-b-" in item
            and f"request_id={request_id} " in item
            and f"peer_public_key={relay} " in item
            and "operation=server.social.ping.deliver result=success" in item), None)
        if routed is not None and acknowledged is not None:
            evidence = {
                "source_peer": source, "source_server": source_server, "source_ingress_edge": source_edge,
                "local_peer": local, "local_server": local_server,
                "target_peer": target, "target_server": target_server, "target_ingress_edge": ingress,
                "relay_edge": relay, "request_id": request_id,
                "source_delivery": line, "edge_route": routed, "target_device_ack": acknowledged,
            }
            Path(sys.argv[3]).write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
            print(f"verified cross-Server notification via different Edge; correlated request_id={request_id}")
            return
    raise SystemExit("missing correlated source relay, Edge route and target ACK evidence")


if __name__ == "__main__":
    main()
