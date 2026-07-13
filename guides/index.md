---
layout: false
---

<script setup>
import { onMounted } from "vue";

onMounted(() => {
  const languages = navigator.languages?.length
    ? navigator.languages
    : [navigator.language];
  const locale = languages.some((language) =>
    language?.toLowerCase().startsWith("zh"),
  )
    ? "zh"
    : "en";

  window.location.replace(`/${locale}/`);
});
</script>

<noscript>
  <a href="/zh/">进入 GizClaw 项目指引</a>
</noscript>
