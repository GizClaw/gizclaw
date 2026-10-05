"""Load the authorized provider key into the runner process, never an artifact."""
import os
from pathlib import Path
import shlex
import subprocess
import sys

values = {}
source_name = os.environ.get("GIZCLAW_RUNTIME_TOOL_CREDENTIAL_FILE")
if not source_name:
    raise SystemExit("GIZCLAW_RUNTIME_TOOL_CREDENTIAL_FILE is required")
source = Path(source_name)
for line in source.read_text().splitlines():
    key, sep, value = line.removeprefix("export ").partition("=")
    if sep and key.strip() == "GIZCLAW_VOLC_ARK_API_KEY":
        parts = shlex.split(value, comments=True)
        if len(parts) != 1:
            raise SystemExit("Invalid provider credential encoding")
        values["GIZCLAW_RUNTIME_TOOL_PROVIDER_KEY"] = parts[0]
if not values.get("GIZCLAW_RUNTIME_TOOL_PROVIDER_KEY"):
    raise SystemExit("Required authorized provider credential is unavailable")
environment = dict(os.environ, **values)
raise SystemExit(subprocess.call(["bash", "-c", Path(sys.argv[1]).read_text(), *sys.argv[1:]], env=environment))
