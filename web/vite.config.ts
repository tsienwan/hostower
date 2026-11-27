import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 构建产物嵌入 Go 二进制。开发时把 /api 代理到本地塔台
// （此时用 HOSTOWER_PUBLIC_URL=http://localhost:5173 启动塔台，来源校验才能通过）。
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist", emptyOutDir: false, sourcemap: false, assetsInlineLimit: 0,
    rollupOptions: {
      output: {
        // 每种语言的词典合成一个文件，按需下载（见 src/i18n.ts）。
        manualChunks(id) {
          const m = /\/locales\/([^/]+)\/[^/]+\.ts$/.exec(id.split("\\").join("/"));
          return m ? "locale-" + m[1] : undefined;
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: { "/api": { target: "http://localhost:8693", ws: true, changeOrigin: false } },
  },
});
