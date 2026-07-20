# Flutter App <Badge type="warning" text="WIP" />

This page explains installation, permissions, connected devices, and common operations of the GizClaw Flutter App.

The App ships a fixed catalog that matches `RuntimeProfile/default`: `doubao-realtime`, four
`translate-*` aliases, `chat`, `journey`, `murder-mystery`, and the internal `chatroom` alias. It does
not call `server.workflow.list` to discover product capabilities. One Workspaces destination lists all
Workspace rows; its single `+` action offers the eight selectable aliases with App-owned ordering,
i18n, icons, and typed creation parameters. Workspace creation uses `source=runtime` and the selected
alias.

Scanning a Desktop local Pod QR stores its raw registration credential in per-Server secure storage
and registers the connection into `RuntimeProfile/default`. The App uses the fixed application token
identity `app:com.gizclaw.opensource`; it does not expose arbitrary RegistrationToken editing or
selection. Rescanning the same Server may replace the stored raw credential after rotation.
