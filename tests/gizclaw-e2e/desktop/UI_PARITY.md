# Desktop UI Parity Map

This document maps the old Go-hosted UI baseline from `origin/main` to the
desktop implementation and e2e coverage for issue #120.

## Baseline Sources

Old Admin UI baseline:

- `origin/main:cmd/ui/admin/app.tsx`
- `origin/main:cmd/ui/admin/layout/*`
- `origin/main:cmd/ui/admin/components/*`
- `origin/main:cmd/ui/admin/pages/overview/*`
- `origin/main:cmd/ui/admin/pages/peers/*`
- `origin/main:cmd/ui/admin/pages/providers/*`
- `origin/main:cmd/ui/admin/pages/ai/*`
- `origin/main:cmd/ui/admin/pages/firmware/*`
- `origin/main:cmd/ui/admin/pages/settings/*`
- `origin/main:cmd/ui/admin/pages/social/*`
- `origin/main:cmd/ui/admin/pages/business/*`
- `origin/main:cmd/ui/admin/pages/resources/*`
- `origin/main:cmd/ui/admin/pages/memory/*`

Old Play UI baseline:

- `origin/main:cmd/ui/play/app.tsx`
- `origin/main:cmd/ui/play/components/*`
- `origin/main:cmd/ui/play/styles.css`
- `origin/main:cmd/ui/play/app_test.go`

Old e2e/user-story baseline:

- `origin/main:tests/gizclaw-e2e/client/admin/*`
- `origin/main:tests/gizclaw-e2e/client/chat/*`
- `origin/main:tests/gizclaw-e2e/client/rpc/*`
- `origin/main:tests/gizclaw-e2e/client/social/*`
- `origin/main:tests/gizclaw-e2e/cmd/*/USER_STORIES.md`

## Desktop Shell

| Old / Required Flow | New Implementation | New E2E Coverage |
| --- | --- | --- |
| UI starts from launcher, not old CLI-hosted Admin/Play URLs | `apps/wails/frontend/src/shell/AppShell.tsx` | `apps/wails/frontend/e2e/shell.spec.ts` |
| Context selection | `apps/wails/frontend/src/shell/AppShell.tsx`, `apps/wails/internal/bridge/context_bridge.go` | `tests/gizclaw-e2e/desktop/shell/context_picker_test.go`, `apps/wails/frontend/e2e/shell.spec.ts` |
| View selection for Admin/Play | `apps/wails/frontend/src/shell/AppShell.tsx`, `apps/wails/internal/bridge/app_bridge.go` | `apps/wails/frontend/e2e/shell.spec.ts` |
| Get Started creates a view session | `apps/wails/internal/bridge/app_bridge.go`, `apps/wails/app.go` | `apps/wails/app_test.go`, `apps/wails/frontend/e2e/shell.spec.ts` |
| Sign out clears only the active session | `apps/wails/internal/bridge/app_bridge.go`, `apps/wails/frontend/src/shell/AppShell.tsx` | `apps/wails/app_test.go`, `apps/wails/frontend/e2e/shell.spec.ts` |
| Private runtime material is not stored in browser storage | `apps/wails/frontend/src/lib/runtime/desktop.ts`, `apps/wails/frontend/src/lib/runtime/types.ts` | `apps/wails/frontend/src/lib/runtime/desktop.test.ts` |

## Admin UI

| Old Admin Area | New Implementation | New E2E Coverage |
| --- | --- | --- |
| Admin layout/sidebar/resource navigation | `apps/wails/frontend/src/views/admin/AdminFullHome.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Peers list/detail | `apps/wails/frontend/src/views/admin/AdminFullHome.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Workflows/workspaces/models/voices | `apps/wails/frontend/src/views/admin/AdminFullHome.tsx`, `apps/wails/frontend/src/lib/gizclaw/admin.ts` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Credentials/provider tenants | `apps/wails/frontend/src/views/admin/AdminFullHome.tsx`, `apps/wails/frontend/src/lib/gizclaw/admin.ts` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Firmware resources | `apps/wails/frontend/src/views/admin/AdminFullHome.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |
| ACL views/roles/policy bindings | `apps/wails/frontend/src/views/admin/AdminFullHome.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Social contacts/friends/friend groups | `apps/wails/frontend/src/views/admin/AdminFullHome.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Gameplay badges/pet species | `apps/wails/frontend/src/views/admin/AdminFullHome.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |

Admin transport mapping:

- Old same-origin Admin HTTP calls are replaced by
  `@gizclaw/gizclaw/admin`.
- Generated Admin API code lives in
  `js/packages/gizclaw/generated/adminservice`.
- WebRTC Admin API fetch transport is implemented in
  `js/packages/gizclaw/index.ts`.

## Play UI

| Old Play Area | New Implementation | New E2E Coverage |
| --- | --- | --- |
| Workspace runtime summary | `apps/wails/frontend/src/views/play/PlayFullHome.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| Workspace set/reload | `apps/wails/frontend/src/views/play/PlayFullHome.tsx`, `apps/wails/frontend/src/lib/gizclaw/play.ts` | `apps/wails/frontend/e2e/play.spec.ts` |
| History list and replay action | `apps/wails/frontend/src/views/play/PlayFullHome.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| Social friend/group resource visibility | `apps/wails/frontend/src/views/play/PlayFullHome.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| Firmware list visibility | `apps/wails/frontend/src/views/play/PlayFullHome.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| Memory stats/recall | `apps/wails/frontend/src/views/play/PlayFullHome.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| Gameplay wallet/reward/pet visibility | `apps/wails/frontend/src/views/play/PlayFullHome.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |

Play transport mapping:

- Old client-service usage is not used.
- Peer RPC calls use `@gizclaw/gizclaw/rpc`.
- Generated peer RPC method/request/response typing lives in
  `js/packages/gizclaw/generated/rpc`.
- The generated RPC method map is produced from
  `api/rpc.json` `x-gizclaw-rpc-methods`.

## Ongoing Regression Focus

The desktop implementation should continue to be checked against the full old UI
surface whenever these areas change:

- Admin page-level forms and create/update/delete dialogs from old per-resource
  pages.
- Admin firmware artifact tree/stat/download detail interactions.
- Play push-to-talk/realtime controls and event stream rendering.
- Chat drawer conversation behavior and active workspace history replay.
- E2E assertions that use real shared setup data in addition to injected browser
  mocks.
