import { defineConfig } from "vitepress";
import { withMermaid } from "vitepress-mermaid-plugin";

const zhDevelopingSidebar = [
  {
    text: "开发指引",
    items: [
      { text: "总览", link: "/zh/developing/" },
      { text: "pkgs/giznet", link: "/zh/developing/giznet" },
      {
        text: "pkgs/gizclaw",
        collapsed: false,
        items: [
          { text: "总览", link: "/zh/developing/gizclaw/overview" },
          {
            text: "peer",
            collapsed: false,
            items: [
              { text: "总览", link: "/zh/developing/gizclaw/peer/overview" },
              { text: "Management", link: "/zh/developing/gizclaw/peer/manager" },
              { text: "Authorization", link: "/zh/developing/gizclaw/peer/authorizer" },
              { text: "Connection", link: "/zh/developing/gizclaw/peer/conn" },
              {
                text: "Services",
                collapsed: true,
                items: [
                  { text: "总览", link: "/zh/developing/gizclaw/peer/service/overview" },
                  { text: "Core Service", link: "/zh/developing/gizclaw/peer/service/core" },
                  { text: "Peer HTTP · WebRTC", link: "/zh/developing/gizclaw/peer/service/webrtc" },
                  { text: "HTTP Service Entrypoints", link: "/zh/developing/gizclaw/peer/service/public-http" },
                  { text: "Peer HTTP · /me", link: "/zh/developing/gizclaw/peer/service/peer-http-me" },
                  { text: "Admin HTTP · Resources", link: "/zh/developing/gizclaw/peer/service/admin-resources" },
                  { text: "Admin HTTP · ACL", link: "/zh/developing/gizclaw/peer/service/admin-acl" },
                  { text: "Admin HTTP · Gameplay", link: "/zh/developing/gizclaw/peer/service/admin-gameplay" },
                  { text: "Admin HTTP · Logs", link: "/zh/developing/gizclaw/peer/service/admin-logs" },
                  { text: "Admin HTTP · Social", link: "/zh/developing/gizclaw/peer/service/admin-social" },
                  { text: "Admin HTTP · Telemetry", link: "/zh/developing/gizclaw/peer/service/admin-telemetry" },
                ],
              },
              { text: "Agent Host", link: "/zh/developing/gizclaw/peer/agent-host" },
              { text: "Realtime Source", link: "/zh/developing/gizclaw/peer/realtime-source" },
              { text: "Stream Events", link: "/zh/developing/gizclaw/peer/stream-event" },
            ],
          },
          {
            text: "server",
            collapsed: false,
            items: [
              { text: "总览", link: "/zh/developing/gizclaw/server/overview" },
              { text: "Server", link: "/zh/developing/gizclaw/server/main" },
              { text: "Log Query", link: "/zh/developing/gizclaw/server/log-query" },
              { text: "OpenAI HTTP", link: "/zh/developing/gizclaw/server/openai-http" },
              { text: "Private HTTP", link: "/zh/developing/gizclaw/server/private-http" },
              { text: "Security Policy", link: "/zh/developing/gizclaw/server/security-policy" },
            ],
          },
          {
            text: "rpc",
            collapsed: true,
            items: [
              { text: "总览", link: "/zh/developing/gizclaw/rpc/overview" },
              { text: "Common", link: "/zh/developing/gizclaw/rpc/all" },
              { text: "Client", link: "/zh/developing/gizclaw/rpc/client" },
              { text: "Server", link: "/zh/developing/gizclaw/rpc/server" },
              { text: "Firmware Download", link: "/zh/developing/gizclaw/rpc/firmware" },
              { text: "Gameplay Assets", link: "/zh/developing/gizclaw/rpc/gameplay-pixa" },
              { text: "Workspace History", link: "/zh/developing/gizclaw/rpc/workspace-history" },
              { text: "Speed Test", link: "/zh/developing/gizclaw/rpc/speed" },
              { text: "Streaming", link: "/zh/developing/gizclaw/rpc/stream" },
              { text: "Tool Invocation", link: "/zh/developing/gizclaw/rpc/tool" },
              { text: "Utilities", link: "/zh/developing/gizclaw/rpc/utils" },
              { text: "Edge Routing", link: "/zh/developing/gizclaw/rpc/edge" },
            ],
          },
          { text: "migrator", link: "/zh/developing/gizclaw/migrator" },
          {
            text: "services",
            collapsed: false,
            items: [
              { text: "总览", link: "/zh/developing/gizclaw/services/overview" },
              { text: "AI", link: "/zh/developing/gizclaw/services/ai" },
              { text: "Device", link: "/zh/developing/gizclaw/services/device" },
              { text: "Gameplay", link: "/zh/developing/gizclaw/services/gameplay" },
              { text: "Runtime", link: "/zh/developing/gizclaw/services/runtime" },
              { text: "Social", link: "/zh/developing/gizclaw/services/social" },
              { text: "System", link: "/zh/developing/gizclaw/services/system" },
            ],
          },
          { text: "generated", link: "/zh/developing/gizclaw/api" },
          { text: "contextstore", link: "/zh/developing/gizclaw/contextstore" },
          { text: "customid", link: "/zh/developing/gizclaw/customid" },
        ],
      },
      { text: "pkgs/gizedge", link: "/zh/developing/gizedge" },
    ],
  },
];

export default withMermaid(
  defineConfig({
    title: "GizClaw Project Guide",
    description: "GizClaw development and usage documentation",
    base: process.env.VITEPRESS_BASE ?? "/",
    cleanUrls: true,
    lastUpdated: true,
    locales: {
      zh: {
        label: "简体中文",
        lang: "zh-CN",
        link: "/zh/",
      },
      en: {
        label: "English",
        lang: "en-US",
        link: "/en/",
      },
    },
    mermaid: {
      theme: "default",
    },
    themeConfig: {
      // English pages are intentionally not mirrored yet. Until they are,
      // language switching must land on a locale home instead of constructing
      // a non-existent corresponding page.
      i18nRouting: (_data, hash, targetLocale) => {
        const localeRoot = targetLocale === "root" ? "/" : `/${targetLocale}/`;
        return `${localeRoot}${hash}`;
      },
      nav: [
        { text: "开发指引", link: "/zh/developing/" },
        { text: "E2E 指引", link: "/zh/e2e/" },
        { text: "代码审核", link: "/zh/reviewing/" },
        { text: "使用说明", link: "/zh/user-guide/" },
        { text: "当前问题", link: "/zh/current-worktree-issues" },
      ],
      sidebar: {
        "/zh/developing/": zhDevelopingSidebar,
        "/zh/e2e/": [
          {
            text: "E2E 指引",
            items: [{ text: "总览", link: "/zh/e2e/" }],
          },
        ],
        "/zh/reviewing/": [
          {
            text: "代码审核",
            items: [{ text: "总览", link: "/zh/reviewing/" }],
          },
        ],
        "/zh/user-guide/": [
          {
            text: "使用说明",
            items: [{ text: "总览", link: "/zh/user-guide/" }],
          },
        ],
      },
      outline: {
        label: "本页目录",
        level: [2, 4],
      },
      docFooter: {
        prev: "上一页",
        next: "下一页",
      },
      lastUpdated: {
        text: "最后更新",
      },
      locales: {
        zh: {
          label: "简体中文",
        },
        en: {
          label: "English",
          nav: [{ text: "Home", link: "/en/" }],
        },
      },
      search: {
        provider: "local",
      },
    },
  }),
);
