# Flutter App <Badge type="warning" text="WIP" />

This page explains installation, permissions, connected devices, and common operations of the GizClaw Flutter App.

The App owns the fixed `assistants`, `translates`, `raids`, `story-teller`, and `role-play`
navigation Collections. It calls `server.workflow.list` with one required Collection at a time and
renders the compatible dynamic Workflow aliases supplied by the active RuntimeProfile. Selecting a
Workflow creates a Workspace with that `collection` and `workflow_alias`, then enters it directly;
the App does not ask the user to select a concrete Model or Voice.

When adding a Server, users can enter an optional registration token or scan a Desktop local Pod QR
that contains one. The App stores tokens per Server in secure storage and registers the connection
into `RuntimeProfile/default`. The App uses the fixed application token identity
`app:com.gizclaw.opensource`. Rescanning the same Server may replace the stored token after Desktop
updates the resource.
