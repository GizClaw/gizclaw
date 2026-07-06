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
| Admin layout/sidebar/resource navigation | `apps/wails/frontend/src/views/admin/full/layout`, `apps/wails/frontend/src/views/admin/full/router.tsx`, `apps/wails/frontend/src/views/admin/AdminFullHome.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Overview dashboard | `apps/wails/frontend/src/views/admin/full/pages/overview/OverviewPage.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Peers list/detail | `apps/wails/frontend/src/views/admin/full/pages/peers` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Workflows/workspaces/models/voices | `apps/wails/frontend/src/views/admin/full/pages/ai`, `apps/wails/frontend/src/lib/gizclaw/admin.ts` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Credentials/provider tenants | `apps/wails/frontend/src/views/admin/full/pages/providers`, `apps/wails/frontend/src/lib/gizclaw/admin.ts` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Firmware list/create/detail/artifact browsing | `apps/wails/frontend/src/views/admin/full/pages/firmware` | `apps/wails/frontend/e2e/admin.spec.ts` |
| ACL views/roles/policy bindings | `apps/wails/frontend/src/views/admin/full/pages/settings` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Social contacts/friends/friend groups/history audio | `apps/wails/frontend/src/views/admin/full/pages/social` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Gameplay badges/pet species | `apps/wails/frontend/src/views/admin/full/pages/business` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Resource manager page | `apps/wails/frontend/src/views/admin/full/pages/resources/ResourcesPage.tsx` | `apps/wails/frontend/e2e/admin.spec.ts` |
| Legacy memory placeholder | `apps/wails/frontend/src/views/admin/full/pages/memory/MemoryPage.tsx` | Not a parity gate: the old `origin/main` router also did not expose this placeholder page |

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
| Workspace runtime summary | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| Workspace set/reload/mode | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx`, `apps/wails/frontend/src/views/play/full/peer-rpc-adapter.ts` | `apps/wails/frontend/e2e/play.spec.ts` |
| Push-to-talk / realtime workspace controls | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx` | `apps/wails/frontend/e2e/play.spec.ts`, `tests/gizclaw-e2e/go/chat` live workspace tests |
| Event stream rendering and active chat history replay | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| History list and replay action | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| Social friend/group resources and chat targets | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx` | `apps/wails/frontend/e2e/play.spec.ts`, `tests/gizclaw-e2e/go/social` |
| Firmware list/detail/download | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx` | `apps/wails/frontend/e2e/play.spec.ts`, `tests/gizclaw-e2e/go/rpc/server_firmware_test.go` |
| Memory stats/recall/reload | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx` | `apps/wails/frontend/e2e/play.spec.ts` |
| Gameplay wallet/reward/pet visibility and actions | `apps/wails/frontend/src/views/play/full/PlayFullApp.tsx` | `apps/wails/frontend/e2e/play.spec.ts`, `tests/gizclaw-e2e/go/rpc/server_pet_test.go`, `tests/gizclaw-e2e/go/rpc/server_reward_test.go`, `tests/gizclaw-e2e/go/rpc/server_wallet_test.go` |

Play transport mapping:

- Old client-service usage is not used.
- Peer RPC calls use `@gizclaw/gizclaw/rpc`.
- Generated peer RPC method/request/response typing lives in
  `js/packages/gizclaw/generated/rpc`.
- The generated RPC method map is produced from
  `api/rpc.json` `x-gizclaw-rpc-methods`.

## Final Acceptance Evidence

The #120 desktop UI redo is considered complete when the following evidence is
current:

- `api/desktop_service.json` defines the local desktop API for contexts, views,
  and view sessions. The generated frontend types live under
  `apps/wails/frontend/src/generated/desktopservice`.
- The desktop launcher uses `apps/wails/frontend/src/lib/runtime/desktop.ts`
  and generated DTO aliases from `apps/wails/frontend/src/lib/runtime/types.ts`.
- The desktop app starts on `AppShell` Welcome/context/view selection and only
  requests injected private runtime material after `Get Started`.
- Dashboard sign-out calls the local desktop API to clear the active session and
  returns to Welcome without deleting stored contexts or identities.
- `@gizclaw/gizclaw` is the only GizClaw JavaScript SDK package used by the
  desktop frontend. Generated Admin, server-public, and peer RPC code lives
  under `js/packages/gizclaw/generated`.
- Admin uses generated Admin API clients through the WebRTC fetch transport in
  `@gizclaw/gizclaw/admin`.
- Play uses generated typed peer RPC through `@gizclaw/gizclaw/rpc`; it does not
  use the removed client-service API.
- Old Go-hosted UI and proxy surfaces remain deleted:
  `cmd/ui/admin`, `cmd/ui/play`, old Play `cmd/internal/clientapi`
  dependencies, and old `cmd/internal/cmdhttp` UI proxy handlers are absent.
- `npm --prefix apps/wails/frontend run test:e2e`,
  `go test -tags gizclaw_e2e -count=1 ./tests/gizclaw-e2e/desktop/...`, and the
  default Docker-backed `bash tests/gizclaw-e2e/run_tests.sh` must pass before
  #109 is closed.
