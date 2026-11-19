import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";
import { createRequire } from "node:module";
import { cpSync, createReadStream, existsSync, statSync } from "node:fs";
import { dirname, join, normalize } from "node:path";
import type { Plugin } from "vite";

// O pdf.js decodifica imagens JPEG2000/JBIG2 e perfis de cor com módulos WebAssembly, e precisa também de fontes e mapas de
// caracteres. Sem esses arquivos, PDFs escaneados (certificados, documentos fotografados) abrem como uma folha em branco.
// Eles saem de node_modules para /pdfjs/ (no dev e no build) e o visualizador aponta para lá.
const PDFJS_FILES = ["wasm", "cmaps", "standard_fonts", "iccs"];
function pdfjsAssets(): Plugin {
  const root = dirname(createRequire(import.meta.url).resolve("pdfjs-dist/package.json"));
  let outDir = "dist";
  return {
    name: "chathm-pdfjs-assets",
    configResolved(c) {
      outDir = join(c.root, c.build.outDir);
    },
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const m = /^\/pdfjs\/([^?]+)/.exec(req.url ?? "");
        const file = m ? normalize(join(root, decodeURIComponent(m[1]))) : "";
        if (!m || !file.startsWith(root) || !PDFJS_FILES.includes(m[1].split("/")[0]) || !existsSync(file) || !statSync(file).isFile()) return next();
        if (file.endsWith(".wasm")) res.setHeader("Content-Type", "application/wasm");
        createReadStream(file).pipe(res);
      });
    },
    writeBundle() {
      for (const d of PDFJS_FILES) cpSync(join(root, d), join(outDir, "pdfjs", d), { recursive: true });
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), pdfjsAssets()],
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
