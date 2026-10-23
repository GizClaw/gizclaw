#!/usr/bin/env bash
# Model-free source-address acceptance through real Docker Server and Edge.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
cd "$repo_dir"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/peer-address-XXXXXX")"
export GIZCLAW_MONITOR_STATE="$run_dir/runtime"
export GIZCLAW_MONITOR_REPORTS="$run_dir/reports"
project="gizclaw-peer-address-$(date +%s)-$$"
export GIZCLAW_MONITOR_IMAGE="$project"
mkdir -p "$GIZCLAW_MONITOR_STATE" "$GIZCLAW_MONITOR_REPORTS"
touch "$run_dir/empty.env"
cat >"$run_dir/compose.yaml" <<'YAML'
services:
  server:
    environment: {NO_PROXY: "*", no_proxy: "*"}
  edge:
    environment: {NO_PROXY: "*", no_proxy: "*"}
  seed:
    environment: {NO_PROXY: "*", no_proxy: "*"}
  test:
    entrypoint: [sleep, infinity]
    command: []
    environment:
      XDG_CONFIG_HOME: /contexts
      NO_PROXY: "*"
      no_proxy: "*"
    volumes:
      - "${GIZCLAW_MONITOR_STATE}/config-home:/contexts"
YAML
compose=(docker compose --env-file "$run_dir/empty.env" -p "$project" -f "$script_dir/docker/compose.monitor.yaml" -f "$run_dir/compose.yaml")
cleanup() {
  local status=$?
  trap - EXIT
  docker rm -f "$project-build" >/dev/null 2>&1 || true
  "${compose[@]}" logs --no-color >"$GIZCLAW_MONITOR_REPORTS/containers.log" 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker image rm "$GIZCLAW_MONITOR_IMAGE" >/dev/null 2>&1 || true
  rm -rf "$GIZCLAW_MONITOR_STATE" "$run_dir/image"
  echo "Peer address E2E reports: $GIZCLAW_MONITOR_REPORTS"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
# shellcheck source=setup/docker-base.sh
# shellcheck disable=SC1091
source "$script_dir/setup/docker-base.sh"
base_image=""
build_gizclaw_e2e_base "$repo_dir" "linux/$arch"
npm ci && npm run build:console
image_dir="$run_dir/image"
mkdir -p "$image_dir/bin" "$image_dir/tests/gizclaw-e2e/docker" "$image_dir/tests/gizclaw-e2e/testdata"
docker run --rm --name "$project-build" --cpus=4 --memory=6g --entrypoint /bin/bash \
  -v "$repo_dir:/src" -v "$image_dir/bin:/out" \
  -v "gizclaw-monitor-modcache:/gomod" -v "gizclaw-monitor-buildcache:/cache" \
  -e GOMODCACHE=/gomod -e GOCACHE=/cache -e GOMAXPROCS=4 -e 'GOFLAGS=-p=4 -buildvcs=false' \
  "$base_image" -lc 'cd /src \
    && go build -o /out/gizclaw ./cmd/gizclaw \
    && go build -o /out/monitor-seed ./tests/gizclaw-e2e/cmd/multiserver-seed \
    && go build -o /out/monitor-fixture ./tests/gizclaw-e2e/cmd/monitor-fixture \
    && go build -o /out/log-sink ./tests/gizclaw-e2e/cmd/log-sink'
python3 - "$image_dir/bin/gizclaw" "$GIZCLAW_MONITOR_REPORTS/build.json" <<'PY'
import hashlib, json, pathlib, subprocess, sys
record = {
    "source_revision": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
    "source_dirty": bool(subprocess.check_output(["git", "status", "--porcelain"], text=True).strip()),
    "gizclaw_sha256": hashlib.sha256(pathlib.Path(sys.argv[1]).read_bytes()).hexdigest(),
}
pathlib.Path(sys.argv[2]).write_text(json.dumps(record, indent=2) + "\n")
PY
cp -R "$script_dir/docker/monitor" "$image_dir/tests/gizclaw-e2e/docker/"
cp -R "$script_dir/testdata/peer-address" "$image_dir/tests/gizclaw-e2e/testdata/"
docker build --build-arg "BASE_IMAGE=$base_image" -f "$script_dir/docker/Dockerfile.monitor" -t "$GIZCLAW_MONITOR_IMAGE" "$image_dir"
docker run --rm -v "$GIZCLAW_MONITOR_STATE:/state" --entrypoint monitor-fixture "$GIZCLAW_MONITOR_IMAGE" -init /state
python3 - "$GIZCLAW_MONITOR_STATE" <<'PY'
import pathlib, sys
root = pathlib.Path(sys.argv[1])
config = root / "server/config.yaml"
text = config.read_text()
old = "  memory:\n    kind: memory\n"
if text.count(old) != 1:
    raise SystemExit("Monitor fixture's KV storage declaration changed")
config.write_text(text.replace(old, "  memory:\n    kind: badger\n    dir: data/kv\n"))
values = dict(line.split("=", 1) for line in (root / "runtime.env").read_text().splitlines())
admin = root / "config-home/gizclaw/admin/config.yaml"
admin.parent.mkdir(parents=True, mode=0o700)
admin.write_text("identity:\n  private-key: " + values["GIZCLAW_E2E_ADMIN_PRIVATE_KEY"] + "\nserver:\n  endpoint: http://server:9820\n")
admin.chmod(0o600)
(root / "fixture.env").touch(mode=0o600)
PY
"${compose[@]}" up -d --wait server edge
"${compose[@]}" run --rm seed >"$GIZCLAW_MONITOR_REPORTS/seed.log" 2>&1
"${compose[@]}" up -d --wait test
python3 - "$GIZCLAW_MONITOR_REPORTS" "$(git rev-parse HEAD)" "${compose[@]}" <<'PY'
import ipaddress, json, pathlib, re, subprocess, sys, time
reports, revision = pathlib.Path(sys.argv[1]), sys.argv[2]
compose = sys.argv[3:]

def command(*args):
    return subprocess.check_output([*compose, *args], text=True, timeout=120)

def admin(*args):
    return json.loads(command("exec", "-T", "test", "gizclaw", "admin", "peers", *args, "--context", "admin"))

def container_ip(service):
    identity = command("ps", "-q", service).strip()
    value = subprocess.check_output(["docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", identity], text=True).strip()
    return str(ipaddress.ip_address(value))

def offline(public_key, expected_ip, expected_address=None):
    deadline = time.monotonic() + 30
    while True:
        value = admin("runtime", public_key)
        if not value["online"]:
            address = value.get("last_addr", "")
            host, port = address.rsplit(":", 1)
            if host.strip("[]") != expected_ip or not 1 <= int(port) <= 65535:
                raise AssertionError(f"unexpected source endpoint: {address}")
            if expected_address is not None and address != expected_address:
                raise AssertionError("Server restart changed the saved address")
            return address
        if time.monotonic() >= deadline:
            raise AssertionError("Giztest device did not go offline after runner exit")
        time.sleep(0.1)

source_ip, edge_ip = container_ip("test"), container_ip("edge")
if source_ip == edge_ip:
    raise AssertionError("runner and Edge must have different source IPs")
results = {}
for route, endpoint in [("direct", "server:9820"), ("gateway", "edge:9821")]:
    name = "peer-address-" + route
    report = route + ".json"
    subprocess.run([*compose, "exec", "-T", "test", "env", "GIZCLAW_TEST_ENDPOINT=" + endpoint,
                    "GIZCLAW_PEER_ADDRESS_HTTP_ENDPOINT=edge:9821",
                    "GIZCLAW_PEER_ADDRESS_CASE=" + name,
                    "GIZCLAW_PEER_ADDRESS_PATTERN=^" + re.escape(source_ip) + r":[1-9][0-9]*$",
                    "gizclaw", "test", "run", "/src/tests/gizclaw-e2e/testdata/peer-address/runtime.giztest.yaml",
                    "--parallel", "1", "--report", "/reports/" + report], check=True, timeout=120)
    evidence = json.loads((reports / report).read_text())
    if evidence["status"] != "passed" or len(evidence["tasks"]) != 1 or any(step["status"] != "passed" for step in evidence["tasks"][0]["steps"]):
        raise AssertionError("every Giztest step must pass without skips")
    peers = admin("list")
    matches = [peer for peer in peers if peer.get("device", {}).get("name", "").startswith(name + "|")]
    if len(matches) != 1:
        raise AssertionError(f"expected exactly one retained {route} identity")
    public_key = matches[0]["public_key"]
    observed = matches[0]["device"]["name"].split("|", 1)[1]
    results[route] = {"public_key": public_key, "last_addr": offline(public_key, source_ip, observed), "giztest": "passed", "offline": "passed"}

command("restart", "server")
command("up", "-d", "--wait", "server", "edge")
for value in results.values():
    offline(value["public_key"], source_ip, value["last_addr"])
    value["restart"] = "passed"
summary = {"status": "passed", "source_revision": revision, "source_ip": source_ip, "edge_ip": edge_ip, "cases": results}
(reports / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2))
PY
