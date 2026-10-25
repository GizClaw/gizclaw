import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
// Standalone deployments configure their access point in deviceEndpoint.
// Development can also proxy device APIs to a local node.
const deviceProxy = process.env.CONSOLE_DEVICE_PROXY ?? "http://127.0.0.1:9821";
export default defineConfig({
  base: "./",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  // The assistant uses its configured endpoint or the device endpoint.
  server: {
    port: 5174,
    proxy: { "/gizclaw": deviceProxy, "/openai": deviceProxy },
  },
});
