import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// The build output is go:embed-ded into kernel/webui and served by the daemon,
// so it lands directly in kernel/webui/dist. emptyOutDir keeps stale hashed
// assets from accumulating (embed.go lives outside dist, so it is never wiped).
// assetsInlineLimit:0 + no inline scripts let the daemon serve under a strict
// CSP (script-src 'self', no nonce). sourcemap:false keeps the committed dist
// lean and reproducible for the CI in-sync check.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "src") } },
  build: {
    outDir: "../kernel/webui/dist",
    emptyOutDir: true,
    sourcemap: false,
    assetsInlineLimit: 0,
    chunkSizeWarningLimit: 1500,
  },
});
