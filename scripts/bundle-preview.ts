import { resolve } from "node:path";

const root = resolve(import.meta.dir, "..");
const output = process.argv[2] ?? "/tmp/leafmark-preview-bundle.js";
// 单文件预览将所有动态 JS 导入合并；样式与字体由 Python 从 Vite 成品内嵌。
const result = await Bun.build({
  entrypoints: [resolve(root, "src/main.ts")],
  target: "browser",
  format: "esm",
  splitting: false,
  minify: true,
  define: {
    "import.meta.env.MODE": JSON.stringify("preview"),
    "import.meta.env.DEV": "false",
    "import.meta.env.PROD": "true",
    "import.meta.env.BASE_URL": JSON.stringify("./"),
  },
  plugins: [{
    name: "预览样式由 Vite 成品提供",
    setup(build) {
      build.onLoad({ filter: /\.(?:css|woff2?|ttf|otf)$/ }, () => ({ contents: "export default '';", loader: "js" }));
    },
  }],
});
if (!result.success) {
  console.error("单文件预览 JS 打包失败。", ...result.logs);
  process.exit(1);
}
const javascript = result.outputs.find(file => file.kind === "entry-point");
if (!javascript) throw new Error("未生成预览 JS 入口。");
await Bun.write(output, javascript);
console.log(`预览 JS 已打包：${output}（${javascript.size} bytes）`);
