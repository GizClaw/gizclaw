import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(dirname, "src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: false,
    rollupOptions: {
      input: {
        admin: path.resolve(dirname, "admin.html"),
        index: path.resolve(dirname, "index.html"),
        play: path.resolve(dirname, "play.html"),
      },
    },
  },
  server: {
    host: "127.0.0.1",
  },
});
