<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { withBase } from "vitepress";
import "@scalar/api-reference/style.css";

type ApiSource = {
  spec: string;
  title: string;
};

const props = defineProps<{
  sources: ApiSource[];
  standalone?: boolean;
}>();

const container = ref<HTMLElement>();
let disposed = false;
let instance: ReturnType<typeof import("@scalar/api-reference")["createApiReference"]> | undefined;
let createReference: typeof import("@scalar/api-reference")["createApiReference"] | undefined;
let themeObserver: MutationObserver | undefined;

function isDarkMode(): boolean {
  return document.documentElement.classList.contains("dark");
}

function configuration() {
  const darkMode = isDarkMode();

  return {
    _integration: "vue",
    agent: {
      disabled: true,
    },
    darkMode,
    defaultHttpClient: {
      clientKey: "curl",
      targetKey: "shell",
    },
    documentDownloadType: "direct",
    forceDarkModeState: darkMode ? "dark" : "light",
    hideClientButton: true,
    hideDarkModeToggle: true,
    hideTestRequestButton: true,
    hiddenClients: true,
    layout: "modern",
    persistAuth: false,
    showDeveloperTools: "never",
    showOperationId: true,
    showSidebar: true,
    showToolbar: "never",
    sources: props.sources.map((source) => ({
      title: source.title,
      url: withBase(source.spec),
    })),
    telemetry: false,
    theme: "default",
  };
}

function mountReference(): void {
  if (disposed || container.value == null || createReference == null) {
    return;
  }

  instance?.destroy();
  container.value.replaceChildren();
  instance = createReference(container.value, configuration());
}

onMounted(async () => {
  ({ createApiReference: createReference } = await import("@scalar/api-reference"));
  if (disposed) {
    return;
  }
  mountReference();

  themeObserver = new MutationObserver(() => {
    mountReference();
  });
  themeObserver.observe(document.documentElement, {
    attributeFilter: ["class"],
    attributes: true,
  });
});

onBeforeUnmount(() => {
  disposed = true;
  themeObserver?.disconnect();
  instance?.destroy();
  container.value?.replaceChildren();
});
</script>

<template>
  <div
    ref="container"
    :class="[
      'gizclaw-api-reference',
      { 'gizclaw-api-reference--standalone': standalone },
    ]"
  >
    <div class="gizclaw-api-reference-loading">正在加载 API Reference…</div>
  </div>
</template>
