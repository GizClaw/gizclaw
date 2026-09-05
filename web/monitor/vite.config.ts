import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
const proxyTarget = process.env.MONITOR_PROXY ?? "http://127.0.0.1:9821";
export default defineConfig({
  base: "/monitor/",
  plugins: [react(), tailwindcss()],
  build: { emptyOutDir: false },
  server: {
    proxy: {
      "/monitor/api": proxyTarget,
      "/gizclaw": proxyTarget,
    },
  },
});
