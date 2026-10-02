import { afterEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { EditorState, EditorSelection } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { markdown } from "@codemirror/lang-markdown";
import { liveBlocks, MAX_BLOCK_PREVIEW_CHARACTERS, type LiveBlocksOptions } from "../src/live-blocks";
import { liveBlockRanges } from "../src/live-block-ranges";
import { buildLiveMarkdownDecorations, liveMarkdown } from "../src/live-markdown";

const globals = ["window", "document", "MutationObserver", "HTMLElement", "Node", "Window", "getComputedStyle", "requestAnimationFrame", "cancelAnimationFrame"] as const;
const originals = new Map(globals.map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)]));
const mounted: { view: EditorView; dom: JSDOM }[] = [];
afterEach(() => {
  for (const { view, dom } of mounted.splice(0)) { view.destroy(); dom.window.close(); }
  for (const key of globals) {
    const original = originals.get(key);
    if (original) Object.defineProperty(globalThis, key, original);
    else Reflect.deleteProperty(globalThis, key);
  }
});
function mount(source: string, showActiveSyntax = true, options: LiveBlocksOptions = {}) {
  const dom = new JSDOM("<html><body><div id='editor'></div></body></html>", { pretendToBeVisual: true, url: "https://leafmark.test" });
  for (const key of globals) {
    const value = dom.window[key];
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value: typeof value === "function" && ["getComputedStyle", "requestAnimationFrame", "cancelAnimationFrame"].includes(key) ? value.bind(dom.window) : value });
  }
  const view = new EditorView({ parent: dom.window.document.querySelector("#editor")!, state: EditorState.create({
    doc: source, extensions: [markdown(), liveMarkdown(), liveBlocks({ showActiveSyntax, ...options }), EditorState.allowMultipleSelections.of(true)],
  }) });
  mounted.push({ view, dom });
  return { view, dom, root: view.dom };
}
const code = "```js\nconst answer = 42;\n```";

test("真实挂载整块代码 widget：语言、高亮、复制及源码保持不变", async () => {
  const { root, view, dom } = mount(code);
  expect(root.querySelector(".lm-block-language")?.textContent).toBe("js");
  expect(root.querySelector(".hljs-keyword")?.textContent).toBe("const");
  expect(root.querySelector("pre code")?.textContent).toBe("const answer = 42;\n");
  let copied = "";
  Object.defineProperty(dom.window.navigator, "clipboard", { value: { writeText: async (text: string) => { copied = text; } } });
  (root.querySelector('[aria-label="复制代码"]') as HTMLButtonElement).click();
  await Promise.resolve();
  expect(copied).toBe("const answer = 42;\n");
  expect(root.querySelector('[role="status"]')?.textContent).toBe("已复制");
  expect(view.state.doc.toString()).toBe(code);
});

test("复制不可用和权限拒绝均显示错误提示", async () => {
  const { root, dom } = mount(code);
  const button = root.querySelector('[aria-label="复制代码"]') as HTMLButtonElement;
  button.click();
  await Promise.resolve();
  expect(root.querySelector('[role="status"]')?.textContent).toContain("复制失败");
  Object.defineProperty(dom.window.navigator, "clipboard", { value: { writeText: async () => { throw new Error("NotAllowedError"); } } });
  button.click();
  await Promise.resolve();
  expect(button.disabled).toBe(false);
  expect(root.querySelector('[role="status"]')?.textContent).toContain("复制失败");
});

test("点击编辑切回源码，强制预览选项也允许编辑，离开后重新预览", () => {
  const { root, view } = mount(code + "\n\nafter", false);
  (root.querySelector('[aria-label="编辑代码源码"]') as HTMLButtonElement).click();
  expect(Boolean(root.querySelector(".lm-code-preview"))).toBe(false);
  expect(view.state.selection.main.head).toBe(6);
  view.dispatch({ changes: { from: 6, insert: "// 注释\n" } });
  expect(Boolean(root.querySelector(".lm-code-preview"))).toBe(false);
  view.dispatch({ selection: { anchor: view.state.doc.length } });
  expect(Boolean(root.querySelector(".lm-code-preview"))).toBe(true);
});

test("焦点控制光标预览，非空选区覆盖和多选区始终保留源码", async () => {
  const { root, view } = mount(code + "\n\nafter");
  view.focus();
  await new Promise(resolve => setTimeout(resolve, 25));
  expect(Boolean(root.querySelector(".lm-code-preview"))).toBe(false);
  view.dispatch({ selection: { anchor: view.state.doc.length } });
  expect(Boolean(root.querySelector(".lm-code-preview"))).toBe(true);
  view.dispatch({ selection: EditorSelection.create([EditorSelection.range(1, 10), EditorSelection.cursor(view.state.doc.length)]) });
  expect(Boolean(root.querySelector(".lm-code-preview"))).toBe(false);
  const forced = mount(code, false);
  forced.view.dispatch({ selection: { anchor: 0, head: code.length } });
  expect(Boolean(forced.root.querySelector(".lm-code-preview"))).toBe(false);
});

test("顶层公式渲染原生 MathML，错误时保留安全源码", () => {
  const { root } = mount("$$\n\\frac{a}{b}\n$$");
  expect(Boolean(root.querySelector('math[display="block"] mfrac'))).toBe(true);
  expect(Boolean(root.querySelector(".katex-html"))).toBe(false);
  const bad = mount("$$\n\\badcommand{<img src=x onerror=alert(1)>}\n$$");
  expect(bad.root.querySelector('[role="status"]')?.textContent).toContain("公式渲染失败");
  expect(bad.root.querySelector("pre code")?.textContent).toContain("<img");
  expect(Boolean(bad.root.querySelector("img"))).toBe(false);
});

test("未知语言安全退回纯文本，Mermaid 暂显示可编辑代码", () => {
  const { root } = mount("```unknown\n<img src=x onerror=alert(1)>\n```\n\n```mermaid\ngraph TD; A-->B\n```");
  expect(root.querySelectorAll(".lm-code-preview")).toHaveLength(2);
  expect(Boolean(root.querySelector("img"))).toBe(false);
  expect(root.textContent).toContain("mermaid");
});

test("跳过容器、缩进、未闭合块以及代码块内部的公式", () => {
  for (const source of ["> ```js\n> x\n> ```", "- item\n  ```js\n  x\n  ```", "    ```js\n    x\n    ```", "```js\nx", "$$\nx", "> $$\n> x\n> $$", "- $$\n  x\n  $$"]) {
    const { view } = mount(source);
    expect(view.state.facet(liveBlockRanges)).toHaveLength(0);
  }
  const { root } = mount("```text\n$$\nx\n$$\n```");
  expect(root.querySelectorAll(".lm-code-preview")).toHaveLength(1);
  expect(Boolean(root.querySelector("math"))).toBe(false);
});

test("liveMarkdown 不装饰已替换范围，源码编辑恢复行装饰", async () => {
  const { view } = mount(code + "\n\n$$\n**a**\n$$");
  const classes: string[] = [];
  buildLiveMarkdownDecorations(view).between(0, view.state.doc.length, (_, __, value) => { if (value.spec.class) classes.push(value.spec.class); });
  expect(classes).not.toContain("md-code-line");
  expect(classes).not.toContain("md-strong");
  view.focus();
  await new Promise(resolve => setTimeout(resolve, 25));
  const editingClasses: string[] = [];
  buildLiveMarkdownDecorations(view).between(0, view.state.doc.length, (_, __, value) => { if (value.spec.class) editingClasses.push(value.spec.class); });
  expect(editingClasses).toContain("md-code-line");
});

test("销毁 widget 后异步复制回调不再修改 DOM", async () => {
  const { root, view, dom } = mount(code);
  let resolve!: () => void;
  Object.defineProperty(dom.window.navigator, "clipboard", { value: { writeText: () => new Promise<void>(done => { resolve = done; }) } });
  const status = root.querySelector('[role="status"]')!;
  (root.querySelector('[aria-label="复制代码"]') as HTMLButtonElement).click();
  view.destroy();
  resolve();
  await Promise.resolve();
  expect(status.textContent).toBe("");
});


test("注入原生 copyText 使用源码并反馈错误", async () => {
  let copied = "";
  const { root } = mount(code, true, { copyText: async text => { copied = text; } });
  (root.querySelector('[aria-label="复制代码"]') as HTMLButtonElement).click();
  await Promise.resolve();
  expect(copied).toBe("const answer = 42;\n");
  expect(root.querySelector('[role="status"]')?.textContent).toBe("已复制");
  const bad = mount(code, true, { copyText: async () => { throw new Error("原生剪贴板失败"); } });
  (bad.root.querySelector('[aria-label="复制代码"]') as HTMLButtonElement).click();
  await Promise.resolve();
  expect(bad.root.querySelector('[role="status"]')?.textContent).toContain("复制失败");
});

test("行内公式原位 MathML，active 行及选区展示源码", async () => {
  const { root, view } = mount("before $x^2$ after\n\nnext");
  expect(root.querySelectorAll(".lm-inline-math math")).toHaveLength(1);
  expect(root.querySelectorAll(".lm-math-preview")).toHaveLength(0);
  view.focus();
  await new Promise(resolve => setTimeout(resolve, 25));
  expect(root.querySelectorAll(".lm-inline-math")).toHaveLength(0);
  view.dispatch({ selection: { anchor: view.state.doc.length } });
  expect(root.querySelectorAll(".lm-inline-math")).toHaveLength(1);
  view.dispatch({ selection: { anchor: 7, head: 12 } });
  expect(root.querySelectorAll(".lm-inline-math")).toHaveLength(0);
});

test("行内公式跳过代码、链接、转义、跨行、双美元和非法公式", () => {
  for (const source of ['[[文件 $x$|别名]]', '![[文件 $x$.png]]', '`$x$`', '[label $x$](https://x)', String.raw`\$x$`, '$$x$$', '$a\nb$', '$ x $', '$x$2', String.raw`$\badcommand$`]) {
    const { root } = mount(source);
    expect(root.querySelectorAll(".lm-inline-math")).toHaveLength(0);
  }
});

test("超大文档回退源码，选区变化不触发 widget", () => {
  let state = EditorState.create({ doc: code + "\n" + "a".repeat(MAX_BLOCK_PREVIEW_CHARACTERS), extensions: [liveBlocks()] });
  expect(state.facet(liveBlockRanges)).toHaveLength(0);
  state = state.update({ selection: { anchor: 10 } }).state;
  expect(state.facet(liveBlockRanges)).toHaveLength(0);
});
