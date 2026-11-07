import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  server: {
    port: 5173,
    // Mesmo origin no dev: cookies de sessão funcionam sem CORS complicado.
    proxy: {
      "/api": { target: process.env.VITE_API_PROXY ?? "http://localhost:8080", changeOrigin: true, ws: true },
      "/mcp": { target: process.env.VITE_API_PROXY ?? "http://localhost:8080", changeOrigin: true },
      // OAuth do MCP: a tela de consentimento (/oauth/consent) é do app React; o resto é da API.
      "/oauth": {
        target: process.env.VITE_API_PROXY ?? "http://localhost:8080",
        changeOrigin: true,
        bypass: (req) => (req.url?.startsWith("/oauth/consent") ? req.url : undefined),
      },
      "/.well-known": { target: process.env.VITE_API_PROXY ?? "http://localhost:8080", changeOrigin: true },
      "/healthz": { target: process.env.VITE_API_PROXY ?? "http://localhost:8080", changeOrigin: true },
    },
  },
  test: {
    environment: "node",
  },
});
