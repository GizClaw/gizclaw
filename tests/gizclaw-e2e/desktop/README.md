# GizClaw Desktop E2E

This directory contains Wails desktop e2e suites.

Expected suites:

- `shell/`: context picker, runtime injection, startup smoke tests
- `admin/`: Wails-hosted Admin view tests
- `play/`: Wails-hosted Play view tests

The shell suite is active. It runs Wails backend Go tests, frontend runtime
tests, frontend build checks, and Playwright shell behavior through the shared
desktop harness.

Admin and Play suites are added when those views are rewritten into
`apps/wails`.
