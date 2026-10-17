#!/usr/bin/env python3
"""Run the complete screenplay owner without another lane's live identities."""

import os
from pathlib import Path
import subprocess
from typing import Mapping


FULL_CASES = "werewolf murder-mystery poetry journey storyteller"
INHERITED_OPTIONS = {
    "GIZCLAW_E2E_CREDENTIAL_FILE",
    "GIZCLAW_E2E_DOCKER_BASE_IMAGE",
    "GIZCLAW_E2E_MEM0_BASE_IMAGE",
    "GIZCLAW_E2E_DOCKER_RETAIN_LOCAL_IMAGES",
    "GIZCLAW_E2E_SCRIPT_QUALITY_ARTIFACT_DIR",
}


def quality_environment(environ: Mapping[str, str]) -> dict[str, str]:
    result = {
        name: value
        for name, value in environ.items()
        if not name.startswith(("GIZCLAW_E2E_", "GIZCLAW_TEST_"))
        or name in INHERITED_OPTIONS
    }
    result["GIZCLAW_E2E_SCRIPT_QUALITY_CASES"] = FULL_CASES
    return result


def run_lane(repo_root: Path, environ: Mapping[str, str]) -> int:
    return subprocess.call(
        ["bash", str(repo_root / "tests/gizclaw-e2e/run_script_quality_tests.sh")],
        cwd=repo_root,
        env=quality_environment(environ),
    )


if __name__ == "__main__":
    raise SystemExit(run_lane(Path(__file__).resolve().parents[3], os.environ))
