# Desktop UI Boundary Map

## Wails control plane

| Flow | Implementation | Coverage |
| --- | --- | --- |
| Headerless compact Pod grid, Add card, and animated detail presence | `apps/wails/frontend/src/shell/AppShell.tsx` | `apps/wails/frontend/e2e/shell.spec.ts` |
| Automatic Pod IDs and minimal local/remote creation | `internal/bridge`, `frontend/src/shell/AppShell.tsx` | bridge tests and `frontend/e2e/shell.spec.ts` |
| Frameless hide/minimise/maximise controls | `frontend/src/shell/AppShell.tsx` | frontend build and shell E2E |
| Local/remote manifest validation and private projection | `apps/wails/internal/appconfig` | `apps/wails/internal/appconfig/pod_test.go` |
| Local Server lifecycle and bounded logs | `apps/wails/internal/localserver` | Wails Go suite |
| Native `/server-info` reachability | `apps/wails/internal/endpointhealth` | `apps/wails/internal/endpointhealth/prober_test.go` |
| Visible system tray icon and navigation | `apps/wails/internal/tray` | macOS build/manual smoke |
| Secret-free Pod bridge | `apps/wails/internal/bridge`, `api/http/desktop.json` | `apps/wails/app_test.go`, generated TypeScript build |
| Invalid/recoverable Pod card and Server search/filter | `apps/wails/frontend/src/shell/AppShell.tsx` | `apps/wails/frontend/e2e/shell.spec.ts` |
| Shared `en`/`zh-CN` launcher and tray catalogs | `apps/wails/i18n`, `frontend/src/i18n.ts` | Go and frontend catalog tests |

## Browser surfaces

| Flow | Implementation | Coverage |
| --- | --- | --- |
| Loopback-only random listener per Pod and surface | `apps/wails/internal/webui` | `apps/wails/internal/webui/manager_test.go` |
| Fresh one-time same-origin Runtime handoff | `apps/wails/internal/webui`, `frontend/src/browser-entry.tsx` | Go webui tests and frontend storage scan |
| Admin browser application | `frontend/admin.html`, `frontend/src/views/admin` | `frontend/e2e/admin.spec.ts` |
| Play browser application | `frontend/play.html`, `frontend/src/views/play` | `frontend/e2e/play.spec.ts` |

Admin and Play retain their generated WebRTC transports and business pages. They
are no longer Wails WebView routes. The desktop schema contains no response field
for private keys; writable key fields appear only in `PodInputWritable`.
