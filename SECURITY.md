# Security Policy

## Supported Versions

Security updates target the current `main` branch unless a supported release
line is announced separately.

## Reporting a Vulnerability

Please do not report security vulnerabilities through public GitHub issues.

Use GitHub private vulnerability reporting for this repository:

https://github.com/GizClaw/gizclaw/security/advisories/new

Include enough detail to reproduce and assess the issue:

- affected component, API, command, or workflow;
- affected commit, release, or deployment shape;
- reproduction steps or proof of concept;
- expected impact and any known mitigations.

We will acknowledge valid reports as soon as practical and coordinate fixes,
release timing, and public disclosure based on severity and exploitability.

## Scope

In scope:

- GizClaw server, CLI, Admin/RPC APIs, and transport code;
- SDK surfaces maintained in this repository;
- workflow runtime integration code;
- firmware and digital content delivery paths;
- repository CI and release automation.

Out of scope:

- vulnerabilities in third-party services unless GizClaw integration code is
  directly responsible for the exposure;
- social engineering, spam, or physical attacks;
- denial-of-service reports without a concrete vulnerability or practical
  mitigation.
