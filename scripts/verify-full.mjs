// 完整原生验收：依次运行 tests/native/ 下的各批步骤，在真实窗口中逐步断言并截图。
// 用法：bun run verify:full [批次…]，批次按文件名匹配，例如 bun run verify:full settings。
import { spawnSync } from "node:child_process";
import { cpSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const suiteDir = join(root, "tests", "native");
const verification = join(root, "verification");
const workspace = join(verification, "full-ws"), workspace2 = join(verification, "full-ws2");
const outRoot = join(verification, "full");

const run = (command, args, options = {}) => spawnSync(command, args, { cwd: root, stdio: "inherit", ...options });
const must = (command, args) => { if (run(command, args).status !== 0) process.exit(1); };

const filters = process.argv.slice(2);
const batches = readdirSync(suiteDir).filter(name => /^\d+-.+\.mjs$/.test(name)).sort()
  .filter(name => !filters.length || filters.some(filter => name.includes(filter)));
if (!batches.length) { console.error("没有匹配的批次：" + filters.join("、")); process.exit(1); }

// 每批都从同一份独立工作区开始，不读写用户笔记。
function resetWorkspace() {
  for (const path of [workspace, workspace2, join(root, ".verification-data")]) rmSync(path, { recursive: true, force: true });
  for (const folder of ["笔记/想法", "项目", "空目录"]) mkdirSync(join(workspace, folder), { recursive: true });
  mkdirSync(workspace2, { recursive: true });
  for (const name of readdirSync(join(root, "tests", "fixtures", "samples"))) {
    if (name.endsWith(".md")) cpSync(join(root, "tests", "fixtures", "samples", name), join(workspace, name));
  }
  writeFileSync(join(workspace, "笔记", "日记.md"), "# 日记\n\n今天写了一点东西。\n");
  writeFileSync(join(workspace, "笔记", "想法", "灵感.md"), "# 灵感\n\n松针落在石阶上。\n");
  writeFileSync(join(workspace, "项目", "Plan-2026.md"), "# Plan 2026\n\n- [ ] one\n");
  writeFileSync(join(workspace, "项目", "周报.md"), "# 周报\n");
  // 非 Markdown 文件用于确认文件树只列出文档。
  writeFileSync(join(workspace, "readme.txt"), "text\n");
  cpSync(join(root, "resources", "icon.png"), join(workspace, "图.png"));
}

must("bun", ["run", "typecheck"]);
must("bunx", ["vite", "build", "--mode", "verification", "--outDir", ".verification-web"]);
// 验证程序只编译一次，放在系统临时目录，结束后清理。
const binDir = mkdtempSync(join(tmpdir(), "leafmark-verify-"));
const binary = join(binDir, "leafmark-verify");
must("go", ["build", "-tags", "verification", "-o", binary, "."]);

const helpers = readFileSync(join(suiteDir, "page-helpers.js"), "utf8");
const summary = [];
try {
  for (const file of batches) {
    const batch = basename(file, ".mjs");
    const out = join(outRoot, batch);
    rmSync(out, { recursive: true, force: true });
    mkdirSync(out, { recursive: true });
    resetWorkspace();
    const build = (await import(pathToFileURL(join(suiteDir, file)).href)).default;
    const steps = build({ workspace, workspace2, out }).map(({ run: body, tap: locate, ...step }) => {
      if (body) return { ...step, js: `${helpers}\nreturn await (${body})();` };
      // 点击步骤：页面里定位元素，取中心点交给原生侧投递鼠标事件。
      if (locate) return { ...step, tap: `${helpers}\nconst target = await (${locate})(); if (!target) throw Error("未找到点击目标"); const box = target.getBoundingClientRect(); return [box.left + box.width / 2, box.top + box.height / 2];` };
      return step;
    });
    const script = join(out, "steps.json");
    writeFileSync(script, JSON.stringify(steps));
    console.log(`\n== ${batch}（${steps.length} 步）==`);
    const status = run(binary, [], { env: { ...process.env, LEAFMARK_VERIFY_SUITE: script, LEAFMARK_VERIFY_OUT: out } }).status;
    let results = [];
    try { results = JSON.parse(readFileSync(join(out, "results.json"), "utf8")); } catch { /* 程序未写出结果，按整批失败处理。 */ }
    const failed = results.filter(step => !step.ok);
    summary.push({ batch, steps: steps.length, ran: results.length, failed: failed.map(step => ({ name: step.name, error: step.error })), passed: status === 0 && results.length === steps.length && !failed.length });
  }
} finally {
  // 只保留结果与截图；工作区和应用数据不留给后续的 verify:native 或官网截图。
  for (const path of [binDir, workspace, workspace2, join(root, ".verification-data")]) rmSync(path, { recursive: true, force: true });
}

const passed = summary.every(item => item.passed);
writeFileSync(join(outRoot, "summary.json"), JSON.stringify({ platform: "macOS WKWebView", passed, batches: summary }, null, 2));
console.log("\n== 汇总 ==");
for (const item of summary) {
  console.log(`${item.passed ? "通过" : "失败"}  ${item.batch}  ${item.ran - item.failed.length}/${item.steps}`);
  for (const step of item.failed) console.log(`      ${step.name}：${String(step.error).split("\n")[0]}`);
}
console.log(`结果与截图：${join("verification", "full")}/<批次>/，汇总见 verification/full/summary.json`);
process.exit(passed ? 0 : 1);
