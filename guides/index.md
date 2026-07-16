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

  const base = import.meta.env.BASE_URL || "/";
  window.location.replace(`${base}${locale}/`);
});
</script>

<noscript>
  <a href="./zh/">简体中文</a>
  ·
  <a href="./en/">English</a>
</noscript>
