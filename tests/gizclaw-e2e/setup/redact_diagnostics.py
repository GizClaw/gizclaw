#!/usr/bin/env python3
"""Redact credentials from bounded GizClaw E2E failure diagnostics."""

from __future__ import annotations

import os
import re
import sys


REDACTED = "[REDACTED]"
SENSITIVE_KEY = (
    r"(?:authorization|api[-_]?key|access[-_]?key(?:[-_]?(?:id|secret))?"
    r"|client[-_]?(?:id|secret)|app[-_]?id|private[-_]?key|password"
    r"|credential|secret|token|signature)"
)
SENSITIVE_ENV_NAME = re.compile(
    r"(?:AUTHORIZATION|API_?KEY|ACCESS_?KEY|CLIENT_?(?:ID|SECRET)|APP_?ID"
    r"|PRIVATE_?KEY|PASSWORD|CREDENTIAL|SECRET|TOKEN|SIGNATURE)",
    re.IGNORECASE,
)
QUOTED_VALUE = re.compile(
    rf"(?i)([\"']?{SENSITIVE_KEY}[\"']?\s*[:=]\s*)([\"'])(.*?)\2"
)
UNQUOTED_VALUE = re.compile(
    rf"(?i)(\b(?:[a-z0-9]+[_-])*{SENSITIVE_KEY}\s*[:=]\s*)"
    rf"(bearer\s+)?([^\s,;&]+)"
)
BEARER_VALUE = re.compile(r"(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]+")
QUERY_VALUE = re.compile(
    r"(?i)([?&](?:access_token|api_key|key|token|signature|sig|x-amz-signature)=)"
    r"[^&\s]+"
)
URL_PASSWORD = re.compile(
    r"(?i)(\b[a-z][a-z0-9+.-]*://[^/\s:@]+:)[^@\s/]+@"
)
PRIVATE_KEY_BLOCK = re.compile(
    r"(-----BEGIN ([A-Z0-9 ]*PRIVATE KEY)-----).*?"
    r"(-----END \2-----)",
    re.DOTALL,
)


def _environment_secrets() -> list[str]:
    secrets = {
        value
        for name, value in os.environ.items()
        if SENSITIVE_ENV_NAME.search(name)
        and len(value) >= 6
        and value != REDACTED
    }
    return sorted(secrets, key=len, reverse=True)


def redact(text: str) -> str:
    for secret in _environment_secrets():
        text = text.replace(secret, REDACTED)
    text = PRIVATE_KEY_BLOCK.sub(
        lambda match: f"{match.group(1)}\n{REDACTED}\n{match.group(3)}",
        text,
    )
    text = QUOTED_VALUE.sub(
        lambda match: f"{match.group(1)}{match.group(2)}{REDACTED}{match.group(2)}",
        text,
    )
    text = UNQUOTED_VALUE.sub(
        lambda match: f"{match.group(1)}{match.group(2) or ''}{REDACTED}",
        text,
    )
    text = BEARER_VALUE.sub(lambda match: f"{match.group(1)}{REDACTED}", text)
    text = QUERY_VALUE.sub(lambda match: f"{match.group(1)}{REDACTED}", text)
    return URL_PASSWORD.sub(lambda match: f"{match.group(1)}{REDACTED}@", text)


def main() -> None:
    sys.stdout.write(redact(sys.stdin.read()))


if __name__ == "__main__":
    main()
