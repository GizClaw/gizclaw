"""Build-only Store injection; production binaries have no control endpoint."""
import difflib
import hashlib
import json
import pathlib
import sys
import subprocess

source, output = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
base = sys.argv[3] if len(sys.argv) > 3 else ""
root = source.parents[2]
sources = {}
for name in ("logger.go", "store_queue.go", "store_failure.go"):
    sources[name] = subprocess.check_output(["git", "-C", str(root), "show", f"{base}:pkgs/gizlog/{name}"]).decode() if base else (source.parent / name).read_text()
text = sources["logger.go"]
anchors = [
    '\t"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"',
    '\tfailureReporter := slog.Handler(',
    '\treturn logger, queue.close, nil',
    '\tlogger := slog.New(newContextHandler(NewFanoutHandler(handlers...), fixed))',
]
if any(text.count(anchor) != 1 for anchor in anchors):
    raise SystemExit("logger wiring changed; review the local probe")
injected = text.replace(anchors[0], anchors[0] + '\n\t"github.com/GizClaw/gizclaw-go/tests/testsupport/logpressure"')
injected = injected.replace(anchors[1], '''\tvar pressure *logpressure.Store
    if os.Getenv("GIZCLAW_E2E_LOG_PRESSURE_ADDRESS") != "" {
        pressure = logpressure.New()
        registry = pressure
        cfg.Sinks = []SinkConfig{{Kind: SinkStderr, Level: cfg.Level}, {Kind: SinkStore, Store: "local-pressure", Level: cfg.Level}}
    }
''' + anchors[1])
injected = injected.replace(anchors[2], '''\tif pressure != nil {
        if err := pressure.Start(os.Getenv("GIZCLAW_E2E_LOG_PRESSURE_ADDRESS"), logger, func() int { return len(queue.jobs) }, cap(queue.jobs)); err != nil {
            return nil, nil, errors.Join(err, queue.close())
        }
        return logger, func() error { return errors.Join(pressure.Close(), queue.close()) }, nil
    }
''' + anchors[2])
injected = injected.replace(anchors[3], '''\tvar root slog.Handler = newContextHandler(NewFanoutHandler(handlers...), fixed)
    if pressure != nil { root = pressure.Observe(root) }
    logger := slog.New(root)''')
output.mkdir(parents=True, exist_ok=True)
(output / "logger.go").write_text(injected)
replacements = {"/src/pkgs/gizlog/logger.go": "/probe/logger.go"}
if base:
    for name in ("store_queue.go", "store_failure.go"):
        (output / name).write_text(sources[name])
        replacements[f"/src/pkgs/gizlog/{name}"] = f"/probe/{name}"
(output / "overlay.json").write_text(json.dumps({"Replace": replacements}))
(output / "logging-source.json").write_text(json.dumps({
    "logging_base_ref": base or None,
    "source_sha256": {name: hashlib.sha256(value.encode()).hexdigest() for name, value in sources.items()},
    "injected_logger_sha256": hashlib.sha256(injected.encode()).hexdigest(),
    "store_fixture_sha256": hashlib.sha256((root / "tests/testsupport/logpressure/store.go").read_bytes()).hexdigest(),
}, indent=2) + "\n")
(output / "injection.patch").write_text("".join(difflib.unified_diff(
    text.splitlines(True), injected.splitlines(True),
    fromfile="a/pkgs/gizlog/logger.go", tofile="b/pkgs/gizlog/logger.go")))
