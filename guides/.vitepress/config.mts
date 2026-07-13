import { cpSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { defineConfig } from "vitepress";
import { withMermaid } from "vitepress-mermaid-plugin";

const zhDevelopingSidebar = [
  {
    text: "开发指引",
    items: [
      { text: "总览", link: "/zh/developing/" },
      {
        text: "API Design",
        collapsed: false,
        items: [
          {
            text: "API",
            collapsed: false,
            items: [
              { text: "总览", link: "/zh/developing/api/overview" },
              {
                text: "HTTP API",
                collapsed: false,
                items: [
                  { text: "总览", link: "/zh/developing/api/http/overview" },
                  { text: "Admin API", link: "/zh/developing/api/http/admin" },
                  { text: "Public API", link: "/zh/developing/api/http/public" },
                  { text: "OpenAI Compatible", link: "/zh/developing/api/http/openai-compatible" },
                  { text: "Shared 与 Resources", link: "/zh/developing/api/http/shared-resources" },
                  { text: "依赖规则", link: "/zh/developing/api/http/type-dependencies" },
                ],
              },
              {
                text: "Proto API",
                collapsed: false,
                items: [
                  { text: "总览", link: "/zh/developing/api/proto/overview" },
                  {
                    text: "Peer RPC",
                    collapsed: false,
                    items: [
                      { text: "总览", link: "/zh/developing/api/proto/rpc/overview" },
                      { text: "Both Provided", link: "/zh/developing/api/proto/rpc/both-provided" },
                      { text: "Client Provided to Server", link: "/zh/developing/api/proto/rpc/client-provided-to-server" },
                      { text: "Server Provided to Client", link: "/zh/developing/api/proto/rpc/server-provided-to-client" },
                      { text: "Server Provided to Edge-node", link: "/zh/developing/api/proto/rpc/server-provided-to-edge-node" },
                    ],
                  },
                  { text: "Telemetry", link: "/zh/developing/api/proto/telemetry" },
                ],
              },
            ],
          },
        ],
      },
      {
        text: "Packages",
        collapsed: false,
        items: [
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
              {
                text: "Runtime",
                collapsed: false,
                items: [
                  { text: "总览", link: "/zh/developing/gizclaw/services/runtime/overview" },
                  { text: "Agent", link: "/zh/developing/gizclaw/services/runtime/agent" },
                  { text: "Agent Host", link: "/zh/developing/gizclaw/services/runtime/agenthost" },
                  { text: "Peer", link: "/zh/developing/gizclaw/services/runtime/peer" },
                  { text: "Peer Resources", link: "/zh/developing/gizclaw/services/runtime/peerresource" },
                  { text: "Peer Route", link: "/zh/developing/gizclaw/services/runtime/peerroute" },
                  { text: "Peer Run", link: "/zh/developing/gizclaw/services/runtime/peerrun" },
                  { text: "Peer Telemetry", link: "/zh/developing/gizclaw/services/runtime/peertelemetry" },
                  { text: "Toolkit", link: "/zh/developing/gizclaw/services/runtime/toolkit" },
                ],
              },
              { text: "Social", link: "/zh/developing/gizclaw/services/social" },
              { text: "System", link: "/zh/developing/gizclaw/services/system" },
            ],
          },
          { text: "generated", link: "/zh/developing/gizclaw/api" },
          { text: "contextstore", link: "/zh/developing/gizclaw/contextstore" },
          { text: "customid", link: "/zh/developing/gizclaw/customid" },
            ],
          },
      {
        text: "pkgs/store",
        collapsed: false,
        items: [
          { text: "总览", link: "/zh/developing/stores/overview" },
          { text: "graph", link: "/zh/developing/stores/graph" },
          { text: "kv", link: "/zh/developing/stores/kv" },
          { text: "metrics", link: "/zh/developing/stores/metrics" },
          { text: "objectstore", link: "/zh/developing/stores/objectstore" },
          { text: "vecid", link: "/zh/developing/stores/vecid" },
          { text: "vecstore", link: "/zh/developing/stores/vecstore" },
        ],
      },
      {
        text: "pkgs/audio",
        collapsed: false,
        items: [
          { text: "总览", link: "/zh/developing/audio/overview" },
          {
            text: "codec",
            collapsed: true,
            items: [
              { text: "mp3", link: "/zh/developing/audio/codec-mp3" },
              { text: "ogg", link: "/zh/developing/audio/codec-ogg" },
              { text: "opus", link: "/zh/developing/audio/codec-opus" },
            ],
          },
          { text: "codecconv", link: "/zh/developing/audio/codecconv" },
          { text: "pcm", link: "/zh/developing/audio/pcm" },
          { text: "portaudio", link: "/zh/developing/audio/portaudio" },
          { text: "resampler", link: "/zh/developing/audio/resampler" },
          { text: "songs", link: "/zh/developing/audio/songs" },
          { text: "voiceprint", link: "/zh/developing/audio/voiceprint" },
        ],
      },
      {
        text: "pkgs/genx",
        collapsed: false,
        items: [
          { text: "总览", link: "/zh/developing/genx/overview" },
          {
            text: "Generators",
            collapsed: false,
            items: [
              { text: "总览", link: "/zh/developing/genx/generators/overview" },
              { text: "OpenAI Adapter", link: "/zh/developing/genx/generators/openai" },
              { text: "Gemini Adapter", link: "/zh/developing/genx/generators/gemini" },
            ],
          },
          {
            text: "Transformers",
            collapsed: false,
            items: [
              { text: "总览", link: "/zh/developing/genx/transformers/overview" },
              { text: "Doubao Speech Adapter", link: "/zh/developing/genx/transformers/doubao" },
              { text: "DashScope Adapter", link: "/zh/developing/genx/transformers/dashscope" },
              { text: "MiniMax Adapter", link: "/zh/developing/genx/transformers/minimax" },
              { text: "Stream Processing", link: "/zh/developing/genx/transformers/stream-processing" },
            ],
          },
          { text: "Segmentors", link: "/zh/developing/genx/segmentors" },
          { text: "Profilers", link: "/zh/developing/genx/profilers" },
          { text: "Labelers", link: "/zh/developing/genx/labelers" },
          { text: "Model Loader", link: "/zh/developing/genx/model-loader" },
          { text: "Match", link: "/zh/developing/genx/match" },
        ],
      },
      { text: "pkgs/gizedge", link: "/zh/developing/gizedge" },
        ],
      },
      { text: "CLI", link: "/zh/developing/cli/" },
      { text: "Wails App", link: "/zh/developing/apps/wails" },
      { text: "Flutter App", link: "/zh/developing/apps/flutter" },
      {
        text: "SDK",
        collapsed: false,
        items: [
          { text: "Go", link: "/zh/developing/sdk/go" },
          { text: "TypeScript", link: "/zh/developing/sdk/typescript" },
          { text: "Flutter", link: "/zh/developing/sdk/flutter" },
        ],
      },
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
    buildEnd(siteConfig) {
      for (const reference of ["flutter", "typescript"]) {
        const source = resolve(siteConfig.srcDir, "references", reference);
        if (existsSync(source)) {
          cpSync(source, resolve(siteConfig.outDir, "references", reference), {
            recursive: true,
          });
        }
      }
    },
    vite: {
      plugins: [
        {
          name: "local-reference-index",
          configureServer(server) {
            server.middlewares.use((request, _response, next) => {
              if (
                request.url?.endsWith("/") &&
                (request.url.startsWith("/references/flutter/") ||
                  request.url.startsWith("/references/typescript/"))
              ) {
                request.url += "index.html";
              }
              next();
            });
          },
        },
      ],
      server: {
        watch: {
          // Codex applies documentation edits through atomic file replacement,
          // which macOS file events may miss. Polling keeps the local guide in sync.
          usePolling: true,
          interval: 300,
          ignored: ["**/.vitepress/dist/**", "**/node_modules/**"],
        },
      },
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
        { text: "审核指引", link: "/zh/reviewing/" },
        { text: "编码规范", link: "/zh/coding-styles/" },
        { text: "使用说明", link: "/zh/using/" },
        { text: "Reference", link: "/references/" },
        { text: "当前问题", link: "/zh/issues" },
      ],
      sidebar: {
        "/zh/developing/": zhDevelopingSidebar,
        "/zh/reviewing/": [
          {
            text: "审核指引",
            items: [
              { text: "总览", link: "/zh/reviewing/" },
              { text: "审查项目", link: "/zh/reviewing/review_items" },
              { text: "开发后自我审查", link: "/zh/reviewing/self_review" },
              { text: "PR Agent 审查", link: "/zh/reviewing/pr_agent_review" },
              { text: "Issue 审查", link: "/zh/reviewing/issue_review" },
            ],
          },
        ],
        "/zh/coding-styles/": [
          {
            text: "编码规范",
            items: [
              { text: "总览", link: "/zh/coding-styles/" },
              { text: "Go", link: "/zh/coding-styles/go" },
              { text: "JavaScript 与 TypeScript", link: "/zh/coding-styles/js" },
              { text: "Dart 与 Flutter", link: "/zh/coding-styles/dart-flutter" },
              { text: "C 与 cgo", link: "/zh/coding-styles/c" },
              { text: "文档", link: "/zh/coding-styles/docs" },
            ],
          },
        ],
        "/zh/using/": [
          {
            text: "使用说明",
            items: [
              { text: "总览", link: "/zh/using/" },
              { text: "CLI", link: "/zh/using/cli" },
              { text: "Wails App", link: "/zh/using/wails-app" },
              { text: "Flutter App", link: "/zh/using/flutter-app" },
              {
                text: "SDK",
                collapsed: false,
                items: [
                  { text: "Go", link: "/zh/using/sdk/go" },
                  { text: "TypeScript", link: "/zh/using/sdk/typescript" },
                  { text: "Flutter", link: "/zh/using/sdk/flutter" },
                ],
              },
            ],
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
