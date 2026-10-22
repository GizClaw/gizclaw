#!/usr/bin/env python3
"""Run the pinned plugin's production-code observer lifecycle fixture."""

import json
from pathlib import Path
import subprocess
import sys
from urllib.parse import unquote, urljoin, urlparse


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: verify-flutter-native-close.py package_config.json")
    config = Path(sys.argv[1]).resolve()
    packages = json.loads(config.read_text())["packages"]
    entry = next((p for p in packages if p.get("name") == "flutter_webrtc"), None)
    if entry is None or not isinstance(entry.get("rootUri"), str):
        raise SystemExit("flutter_webrtc is missing from the package configuration")
    uri = urlparse(urljoin(config.as_uri(), entry["rootUri"]))
    if uri.scheme != "file" or uri.netloc not in ("", "localhost"):
        raise SystemExit("flutter_webrtc must resolve to a local package directory")
    fixture = Path(unquote(uri.path)) / "test/native/run_data_channel_close_lifecycle.py"
    if not fixture.is_file():
        raise SystemExit("the pinned native observer lifecycle fixture is missing")
    subprocess.run([sys.executable, str(fixture)], check=True, timeout=90)


if __name__ == "__main__":
    main()
