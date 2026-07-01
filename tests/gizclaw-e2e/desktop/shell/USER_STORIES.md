# Desktop Shell User Stories

- As a desktop user, I can start the app with an isolated local context store.
- As a desktop user, I can create or select a WebRTC context.
- As a desktop user, my selected context persists in desktop config files, not
  browser storage.
- As a desktop user, the UI receives the selected signaling URL and private key
  material at runtime so WebRTC can connect.
- As a desktop maintainer, shell behavior is covered with Playwright using a
  mock Wails bridge before the Admin and Play views are rewritten.
