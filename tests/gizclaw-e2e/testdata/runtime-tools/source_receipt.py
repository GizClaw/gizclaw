"""Capture the exact Go, protocol and embedded API inputs for a Docker build."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

root = Path(sys.argv[1])
output = Path(sys.argv[2])
paths = subprocess.check_output(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], cwd=root).decode().split("\0")
inputs = {}
for name in sorted(set(paths)):
    path = root / name
    if not name or not path.is_file() or path.is_symlink():
        continue
    if path.suffix not in {".go", ".proto"} and not name.startswith("api/http/") and name not in {"go.mod", "go.sum"}:
        continue
    inputs[name] = hashlib.sha256(path.read_bytes()).hexdigest()
output.write_text(json.dumps(inputs, sort_keys=True, indent=2) + "\n")
