# Desktop Admin User Stories

- As an admin user, I can open the browser-served Admin UI from a Pod Server.
- As an admin user, Admin resources load through the generated Admin API client
  over the WebRTC Admin API transport boundary.
- As an admin user, I can scan resource sections such as peers, workflows,
  workspaces, credentials, voices, firmware, social resources, provider
  tenants, RuntimeProfiles, and RegistrationTokens.
- As a maintainer, the desktop Admin view is tested with Playwright in the
  desktop e2e tree instead of the old CLI-served UI path.
