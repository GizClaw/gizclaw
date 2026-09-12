import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
// The console is served from the access point itself, so device APIs are
// same-origin in production. In development they are proxied to a node.
const deviceProxy = process.env.CONSOLE_DEVICE_PROXY ?? "http://127.0.0.1:9821";
export default defineConfig({
  base: "./",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  // The assistant talks to the node's /openai/v1, same-origin in production.
  server: {
    port: 5174,
    proxy: { "/gizclaw": deviceProxy, "/openai": deviceProxy },
  },
});
