export default [
  {
    input: "../api/admin_service.json",
    output: "packages/gizclaw/generated/adminservice",
  },
  {
    input: "../api/server_public.json",
    output: "packages/gizclaw/generated/serverpublic",
  },
  {
    input: "../api/rpc.json",
    output: "packages/gizclaw/generated/rpc",
  },
  {
    input: "../api/desktop_service.json",
    output: "../apps/wails/frontend/src/generated/desktopservice",
  },
];
