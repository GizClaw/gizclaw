"""Observe the production read guard in the isolated multi-Learner build.

The overlay logs only request type/peer/labels, never keys or values. It does
not change request selection, guard decisions, timestamps or responses.
"""
import difflib
import hashlib
import json
import pathlib
import sys

root, output = map(pathlib.Path, sys.argv[1:])
source = root / "pkgs/store/kv/tikv_read_policy.go"
text = source.read_text()
anchor = "\tif s.isRegionalReadTarget(req) {"
if text.count(anchor) != 1 or text.count('\t"errors"\n') != 1:
    raise SystemExit("read guard changed; review the routing observer")
injected = text.replace('\t"errors"\n', '\t"errors"\n\t"fmt"\n\t"os"\n')
injected = injected.replace(anchor, '''	var actual []*metapb.StoreLabel
	for _, store := range s.client.GetRegionCache().GetAllStores() {
		if store.StoreID() == req.Peer.GetStoreId() { actual = store.GetLabels(); break }
	}
	fmt.Fprintf(os.Stderr, "E2E_TIKV_ROUTE server=%s command=%s region=%d peer=%d store=%d role=%s required=%v actual=%v accepted=%t\\n", os.Getenv("GIZCLAW_E2E_SERVER_ENDPOINT"), req.Type, req.RegionId, req.Peer.GetId(), req.Peer.GetStoreId(), req.Peer.GetRole(), s.labels, actual, s.isRegionalReadTarget(req))
''' + anchor)
output.mkdir(parents=True, exist_ok=True)
(output / "tikv_read_policy.go").write_text(injected)
(output / "overlay.json").write_text(json.dumps({"Replace": {"/src/pkgs/store/kv/tikv_read_policy.go": "/probe/tikv_read_policy.go"}}) + "\n")
(output / "read-routing-source.json").write_text(json.dumps({
    "guard_sha256": hashlib.sha256(text.encode()).hexdigest(),
    "observer_sha256": hashlib.sha256(injected.encode()).hexdigest(),
}, indent=2) + "\n")
(output / "injection.patch").write_text("".join(difflib.unified_diff(text.splitlines(True), injected.splitlines(True), fromfile="a/pkgs/store/kv/tikv_read_policy.go", tofile="b/pkgs/store/kv/tikv_read_policy.go")))
