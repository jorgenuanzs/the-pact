import { fileURLToPath, URL } from "node:url";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  base: "/",
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: fileURLToPath(
      new URL("../internal/transport/httpapi/publicui/dist", import.meta.url),
    ),
    emptyOutDir: true,
    rollupOptions: {
      input: fileURLToPath(new URL("./site.html", import.meta.url)),
    },
  },
  server: {
    port: 5174,
  },
});
