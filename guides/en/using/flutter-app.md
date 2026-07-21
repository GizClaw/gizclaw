# Flutter App <Badge type="warning" text="WIP" />

This page explains installation, permissions, connected devices, and common operations of the GizClaw Flutter App.

The App owns the fixed `assistants`, `translates`, `raids`, `story-teller`, and `role-play`
navigation Collections. It calls `server.workflow.list` with one required Collection at a time and
renders the compatible dynamic Workflow aliases supplied by the active RuntimeProfile. Selecting a
Workflow creates a Workspace with that `collection` and `workflow_alias`, then enters it directly;
the App does not ask the user to select a concrete Model or Voice.

Scanning a Desktop local Pod QR stores its raw registration credential in per-Server secure storage
and registers the connection into `RuntimeProfile/default`. The App uses the fixed application token
identity `app:com.gizclaw.opensource`; it does not expose arbitrary RegistrationToken editing or
selection. Rescanning the same Server may replace the stored raw credential after rotation.
