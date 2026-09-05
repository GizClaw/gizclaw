# GizClaw Monitor design

Reference: https://github.com/VoltAgent/awesome-design-md/blob/main/design-md/claude/DESIGN.md (community Claude design analysis).

Use a warm canvas #faf9f5, ink #141413, muted text #6c6a64, hairlines #e6dfd8 and terracotta accent #cc785c. Use warm dark #181715 for logs. Teal and terracotta distinguish RX and TX curves. Errors are red and never communicated by color alone.

This is a working monitoring console: compact 14px UI, 12px metadata, generous section separation, restrained 28px serif page headings, sans-serif controls and monospaced identifiers/logs. No marketing hero or decorative graphs. Empty and unavailable values must remain explicitly empty. Cards have subtle borders, 12px radii, no heavy shadows. Buttons have visible focus and disabled states.

Node and peer share a 220px navigation rail, compact identity header, four summary metrics, live traffic chart and tabs for logs/configuration/telemetry. Below 800px use horizontal navigation and stacked panels. Authentication stays in memory. Every screen shows connection/error/stale state, refresh control and data provenance. Logs are bounded and virtualized; charts retain 120 samples and stop when paused. Do not load external fonts or assets.
