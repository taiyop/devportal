import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig({
  plugins: [react(), tailwindcss(), wails("./src/bindings")],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  clearScreen: false,
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
    // `wails3 dev` starts Vite with `--port` and already opens the desktop window.
    open: process.argv.slice(2).some((arg) => arg === "--port" || arg.startsWith("--port="))
      ? false
      : true,
    watch: {
      ignored: ["**/src-wails/**"],
    },
  },
});
