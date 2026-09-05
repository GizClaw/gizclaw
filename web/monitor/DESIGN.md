# GizClaw Monitor design

Reference: https://github.com/VoltAgent/awesome-design-md/blob/main/design-md/claude/DESIGN.md (community Claude design analysis).

Use a warm canvas #faf9f5, ink #141413, muted text #6c6a64, hairlines #e6dfd8 and terracotta accent #cc785c. Use warm dark #181715 for logs. Teal and terracotta distinguish RX and TX curves. Errors are red and never communicated by color alone.

This is a working monitoring console: compact 14px UI, 12px metadata, generous section separation, restrained 28px serif page headings, sans-serif controls and monospaced identifiers/logs. No marketing hero or decorative graphs. Empty and unavailable values must remain explicitly empty. Cards have subtle borders, 12px radii, no heavy shadows. Buttons have visible focus and disabled states.

Node and peer share navigation, a compact identity header and explicit connection/error/permission state. Peer tabs separate overview, Workflows and persisted Workspace chat, live traffic, Telemetry, location, runtime logs and configuration. Telemetry uses numeric cards with units and observation times; raw JSON is collapsed. Charts contain actual samples only. Logs use compact single-line records with horizontal overflow, never tables. Location uses reported GNSS coordinates and an OpenStreetMap embed; missing coordinates do not create a marker.

Store connections in origin-local IndexedDB encrypted with a nonextractable Web Crypto key, clear on explicit logout, and explain that same-origin scripts can use that key. This is not an OS keychain. Keep telemetry and logs out of browser persistence. Traffic retains at most 1800 samples with 2/10/30 minute windows, resets its baseline after pause, and clears unavailable data. Below 800px use horizontal navigation and stacked panels. Fonts and application code are bundled; the optional map requires external network access.
