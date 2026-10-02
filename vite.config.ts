import { defineConfig } from "vite";
export default defineConfig(({ mode }) => ({
  // 原生验证关注运行行为，无需承担发布压缩的耗时；发布构建保持默认优化。
  build: mode === "verification" ? { minify: false, reportCompressedSize: false } : undefined,
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    watch: { ignored: ["**/.mygo/**", "**/build/**", "**/verification/**"] },
  },
}));
