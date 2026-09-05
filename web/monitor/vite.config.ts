import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  base: "/monitor/",
  plugins: [react(), tailwindcss()],
  build: { emptyOutDir: false },
  server: {
    proxy: {
      "/monitor/api": "http://127.0.0.1:9821",
      "/gizclaw": "http://127.0.0.1:9821",
    },
  },
});
