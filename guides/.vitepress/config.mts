import { defineConfig } from "vitepress";
import { withMermaid } from "vitepress-mermaid-plugin";

const zhDevelopingSidebar = [
  {
    text: "开发指引",
    items: [
      { text: "总览", link: "/zh/developing/" },
      { text: "Observability", link: "/zh/developing/observability" },
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
              { text: "RuntimeProfile", link: "/zh/developing/gizclaw/services/runtime-profile" },
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
          { text: "memory", link: "/zh/developing/stores/memory" },
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
      { text: "测试与 E2E", link: "/zh/developing/testing" },
      { text: "开发工具与示例", link: "/zh/developing/tooling" },
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

type SidebarEntry = {
  text?: string;
  link?: string;
  collapsed?: boolean;
  items?: SidebarEntry[];
};

const englishSidebarLabels: Record<string, string> = {
  "开发指引": "Development Guide",
  "总览": "Overview",
  "Shared 与 Resources": "Shared and Resources",
  "依赖规则": "Dependency Rules",
  "测试与 E2E": "Testing and E2E",
  "开发工具与示例": "Development Tools and Examples",
};

function englishSidebar(items: SidebarEntry[]): SidebarEntry[] {
  return items.map((item) => ({
    ...item,
    text: item.text ? englishSidebarLabels[item.text] ?? item.text : item.text,
    link: item.link?.replace(/^\/zh\//, "/en/"),
    items: item.items ? englishSidebar(item.items) : undefined,
  }));
}

const enDevelopingSidebar = englishSidebar(zhDevelopingSidebar);

export default withMermaid(
  defineConfig({
    title: "GizClaw Project Guide",
    description: "GizClaw development and usage documentation",
    srcExclude: [
      "zh/reviewing/examples/**",
      "en/reviewing/examples/**",
    ],
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
        themeConfig: {
          nav: [
            { text: "Development", link: "/en/developing/" },
            { text: "Review", link: "/en/reviewing/" },
            { text: "Coding Conventions", link: "/en/coding-styles/" },
            { text: "Usage", link: "/en/using/" },
            { text: "Reference", link: "/references/" },
            { text: "Admin API", link: "/api/" },
          ],
          outline: { label: "On this page", level: [2, 4] },
          docFooter: { prev: "Previous page", next: "Next page" },
          lastUpdated: { text: "Last updated" },
          darkModeSwitchLabel: "Appearance",
          langMenuLabel: "Change language",
          returnToTopLabel: "Return to top",
          sidebarMenuLabel: "Menu",
        },
      },
    },
    mermaid: {
      theme: "default",
    },
    vite: {
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
      // VitePress 2.0.0-alpha.18 passes hash.value as the second argument.
      i18nRouting: ({ page }, hash: string, targetLocale) => {
        const relativePath = page.value.relativePath;
        if (relativePath.startsWith("references/") || relativePath.startsWith("api/")) {
          const referencePath = relativePath
            .replace(/(^|\/)index\.md$/, "$1")
            .replace(/\.md$/, "");
          return `/${referencePath}${hash}`;
        }

        const localizedPath = relativePath
          .replace(/^(zh|en)\//, "")
          .replace(/(^|\/)index\.md$/, "$1")
          .replace(/\.md$/, "");
        const localeRoot = targetLocale === "root" ? "/" : `/${targetLocale}/`;
        return `${localeRoot}${localizedPath}${hash}`;
      },
      nav: [
        { text: "开发指引", link: "/zh/developing/" },
        { text: "审核指引", link: "/zh/reviewing/" },
        { text: "编码规范", link: "/zh/coding-styles/" },
        { text: "使用说明", link: "/zh/using/" },
        { text: "Reference", link: "/references/" },
        { text: "Admin API", link: "/api/" },
      ],
      sidebar: {
        "/references/": [
          {
            text: "Reference",
            items: [
              { text: "总览", link: "/references/" },
              { text: "RPC Methods", link: "/references/rpc" },
              { text: "Events", link: "/references/events" },
              { text: "Streams", link: "/references/streams" },
            ],
          },
        ],
        "/zh/developing/": zhDevelopingSidebar,
        "/en/developing/": enDevelopingSidebar,
        "/zh/reviewing/": [
          {
            text: "审核指引",
            items: [
              { text: "总览", link: "/zh/reviewing/" },
              { text: "Issue 格式", link: "/zh/reviewing/issue-format" },
              { text: "Issue 审查", link: "/zh/reviewing/issue_review" },
              { text: "PR 与 Commit 格式", link: "/zh/reviewing/pr-commit-format" },
              { text: "审查项目", link: "/zh/reviewing/review_items" },
              { text: "开发后自我审查", link: "/zh/reviewing/self_review" },
              { text: "PR Agent 审查", link: "/zh/reviewing/pr_agent_review" },
            ],
          },
        ],
        "/en/reviewing/": [
          {
            text: "Review Guide",
            items: [
              { text: "Overview", link: "/en/reviewing/" },
              { text: "Issue Format", link: "/en/reviewing/issue-format" },
              { text: "Issue Review", link: "/en/reviewing/issue_review" },
              { text: "PR and Commit Format", link: "/en/reviewing/pr-commit-format" },
              { text: "Review Items", link: "/en/reviewing/review_items" },
              { text: "Post-development Self-review", link: "/en/reviewing/self_review" },
              { text: "PR Agent Review", link: "/en/reviewing/pr_agent_review" },
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
        "/en/coding-styles/": [
          {
            text: "Coding Conventions",
            items: [
              { text: "Overview", link: "/en/coding-styles/" },
              { text: "Go", link: "/en/coding-styles/go" },
              { text: "JavaScript and TypeScript", link: "/en/coding-styles/js" },
              { text: "Dart and Flutter", link: "/en/coding-styles/dart-flutter" },
              { text: "C and cgo", link: "/en/coding-styles/c" },
              { text: "Documentation", link: "/en/coding-styles/docs" },
            ],
          },
        ],
        "/zh/using/": [
          {
            text: "使用说明",
            items: [
              { text: "总览", link: "/zh/using/" },
              { text: "API", link: "/zh/using/api" },
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
        "/en/using/": [
          {
            text: "Usage Guide",
            items: [
              { text: "Overview", link: "/en/using/" },
              { text: "API", link: "/en/using/api" },
              { text: "CLI", link: "/en/using/cli" },
              { text: "Wails App", link: "/en/using/wails-app" },
              { text: "Flutter App", link: "/en/using/flutter-app" },
              {
                text: "SDK",
                collapsed: false,
                items: [
                  { text: "Go", link: "/en/using/sdk/go" },
                  { text: "TypeScript", link: "/en/using/sdk/typescript" },
                  { text: "Flutter", link: "/en/using/sdk/flutter" },
                ],
              },
            ],
          },
        ],
      },
      socialLinks: [
        { icon: "github", link: "https://github.com/GizClaw/gizclaw" },
      ],
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
      search: {
        provider: "local",
        options: {
          locales: {
            zh: {
              translations: {
                button: {
                  buttonText: "搜索",
                  buttonAriaLabel: "搜索文档",
                },
                modal: {
                  displayDetails: "显示详细列表",
                  resetButtonTitle: "清除查询条件",
                  backButtonTitle: "关闭搜索",
                  noResultsText: "没有找到相关结果",
                  footer: {
                    selectText: "选择",
                    selectKeyAriaLabel: "回车键",
                    navigateText: "切换",
                    navigateUpKeyAriaLabel: "向上箭头",
                    navigateDownKeyAriaLabel: "向下箭头",
                    closeText: "关闭",
                    closeKeyAriaLabel: "Escape 键",
                  },
                },
              },
            },
            en: {
              translations: {
                button: {
                  buttonText: "Search",
                  buttonAriaLabel: "Search documentation",
                },
                modal: {
                  displayDetails: "Display detailed results",
                  resetButtonTitle: "Reset search",
                  backButtonTitle: "Close search",
                  noResultsText: "No results found",
                  footer: {
                    selectText: "Select",
                    selectKeyAriaLabel: "Enter key",
                    navigateText: "Navigate",
                    navigateUpKeyAriaLabel: "Up arrow",
                    navigateDownKeyAriaLabel: "Down arrow",
                    closeText: "Close",
                    closeKeyAriaLabel: "Escape key",
                  },
                },
              },
            },
          },
        },
      },
    },
  }),
);
