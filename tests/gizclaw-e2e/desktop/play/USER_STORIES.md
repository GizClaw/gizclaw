# Desktop Play User Stories

- As a play user, I can open the browser-served Play UI from a Pod.
- As a play user, workspace runtime state loads through direct WebRTC RPC,
  without the old client-service HTTP API.
- As a play user, I can refresh and reload the active workspace and request a
  workspace switch through direct RPC.
- As a play user, I can inspect workspace history and request replay through
  `server.run.workspace.history.play`.
- As a play user, I can scan social resources and firmware resources that load
  through direct RPC helper methods.
- As a maintainer, the desktop Play view is tested with Playwright in the
  desktop e2e tree instead of the old CLI-served UI path.
