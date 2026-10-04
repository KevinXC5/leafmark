import { afterEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { markdown } from "@codemirror/lang-markdown";
import { GFM } from "@lezer/markdown";
import { closeSearchPanel, openSearchPanel, search, SearchQuery, setSearchQuery } from "@codemirror/search";
import { liveBlocks } from "../../../src/editor/live/live-blocks";
import { liveTable } from "../../../src/editor/live/live-table";
import { liveMarkdown } from "../../../src/editor/live/live-markdown";
import { sourceHighlighting } from "../../../src/editor/source-highlighting";

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
function mount(source: string, source_mode = false) {
  const dom = new JSDOM("<html><body><div id='editor'></div></body></html>", { pretendToBeVisual: true, url: "https://leafmark.test" });
  for (const key of globals) {
    const value = dom.window[key];
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value: typeof value === "function" && ["getComputedStyle", "requestAnimationFrame", "cancelAnimationFrame"].includes(key) ? value.bind(dom.window) : value });
  }
  const view = new EditorView({ parent: dom.window.document.querySelector("#editor")!, state: EditorState.create({
    doc: source, extensions: [markdown({ extensions: [GFM] }), search({ top: true }), source_mode ? sourceHighlighting() : [liveMarkdown(), liveTable(), liveBlocks()]],
  }) });
  mounted.push({ view, dom });
  return { view, dom, root: view.dom };
}

const table = "| 名称 | 说明 |\n| --- | --- |\n| **粗** | [官网](https://example.com) 与 `code` |\n| ==亮== | ![图](https://example.com/a.png) <b>x</b> a \\| b |";

test("表格预览渲染单元格内的行内格式，图片只显示替代文本", () => {
  const { root } = mount(table);
  const cells = [...root.querySelectorAll(".lm-table-preview td")];
  expect(cells[0]!.querySelector("strong")?.textContent).toBe("粗");
  expect(cells[1]!.querySelector("a")?.getAttribute("href")).toBe("https://example.com");
  expect(cells[1]!.querySelector("code")?.textContent).toBe("code");
  expect(cells[2]!.querySelector("mark")?.textContent).toBe("亮");
  // 图片不创建 img 元素，不会发起请求；原始 HTML 作为文本显示，转义的管道符还原。
  expect(root.querySelector(".lm-table-preview img")).toBeNull();
  expect(cells[3]!.querySelector(".lm-image-fallback")?.textContent).toBe("图");
  expect(cells[3]!.querySelector("b")).toBeNull();
  expect(cells[3]!.textContent).toContain("<b>x</b> a | b");
});

test("表格菜单移除时同步失焦不会重复删除，插行仍写入文档", () => {
  const { root, view, dom } = mount("| A |\n| --- |\n| x |");
  root.querySelector<HTMLButtonElement>(".lm-table-actions")!.click();
  const menu = root.querySelector<HTMLElement>('[role="menu"]')!;
  const remove = menu.remove.bind(menu);
  let removals = 0;
  // 模拟 WebView2 在移除聚焦节点过程中同步派发 focusout。
  menu.remove = () => {
    if (++removals > 1) throw new Error("菜单被重入删除");
    menu.dispatchEvent(new dom.window.FocusEvent("focusout", { bubbles: true, relatedTarget: null }));
    remove();
  };
  menu.querySelector<HTMLButtonElement>("button")!.click();
  expect(removals).toBe(1);
  expect(root.querySelector('[role="menu"]')).toBeNull();
  expect(view.state.doc.toString()).toBe("| A |\n| --- |\n|  |\n| x |");
});

test("表格预览中的危险链接不生成可点击地址", () => {
  const { root } = mount("| A |\n| --- |\n| [x](javascript:alert(1)) |");
  expect(root.querySelector(".lm-table-preview a")).toBeNull();
  expect(root.querySelector(".lm-table-preview td")?.textContent).toContain("javascript:alert(1)");
});

test("查找命中预览内的文字时显示源文，关闭面板后恢复预览", () => {
  const { root, view } = mount(`${table}\n\n正文\n\n\`\`\`js\nconst code = 1;\n\`\`\`\n\n尾`);
  const previews = () => ({ tables: root.querySelectorAll(".lm-table-preview").length, blocks: root.querySelectorAll(".lm-code-preview").length });
  expect(previews()).toEqual({ tables: 1, blocks: 1 });
  openSearchPanel(view);
  view.dispatch({ effects: setSearchQuery.of(new SearchQuery({ search: "code" })) });
  expect(previews()).toEqual({ tables: 0, blocks: 0 });
  view.dispatch({ effects: setSearchQuery.of(new SearchQuery({ search: "官网" })) });
  expect(previews()).toEqual({ tables: 0, blocks: 1 });
  view.dispatch({ effects: setSearchQuery.of(new SearchQuery({ search: "正文" })) });
  expect(previews()).toEqual({ tables: 1, blocks: 1 });
  view.dispatch({ effects: setSearchQuery.of(new SearchQuery({ search: "code" })) });
  closeSearchPanel(view);
  expect(previews()).toEqual({ tables: 1, blocks: 1 });
});

test("源码模式给围栏代码的每一行加等宽样式", () => {
  const { root } = mount("正文\n\n```js\nconst a = 1;\n```\n\n尾", true);
  const lines = [...root.querySelectorAll(".cm-line")].map(line => [line.textContent, line.classList.contains("md-source-code")]);
  expect(lines).toEqual([["正文", false], ["", false], ["```js", true], ["const a = 1;", true], ["```", true], ["", false], ["尾", false]]);
});
