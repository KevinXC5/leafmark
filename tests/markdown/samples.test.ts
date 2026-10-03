import { beforeAll, describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { JSDOM } from "jsdom";
import { renderMarkdown } from "../../src/markdown/render-markdown";

beforeAll(() => {
  Object.defineProperty(globalThis, "window", { value: new JSDOM("").window, configurable: true });
});

// 示例文档同时用于官网截图，语法覆盖面缩水时在此处发现。
function render(name: string) {
  const window = new JSDOM("").window;
  window.document.body.innerHTML = renderMarkdown(readFileSync(new URL(`../fixtures/samples/${name}`, import.meta.url), "utf8"));
  return window.document.body;
}

describe("示例文档", () => {
  test("山中来信：行内样式、引用、提示块、列表、公式、表格与代码块", () => {
    const body = render("山中来信.md");
    for (const selector of ["h1", "h2", "strong", "em", "s", "blockquote:not(.callout)", "blockquote.callout", "ol > li math", "ul ul > li", "li code", "mark", "a[href]", "table", "pre code.language-go"]) {
      expect(body.querySelector(selector), selector).not.toBeNull();
    }
    const boxes = [...body.querySelectorAll<HTMLInputElement>("li.task-list-item input[type=checkbox]")];
    expect(boxes.map(box => box.checked)).toEqual([true, false]);
  });
  test("叶脉笔记：提示块、公式、上下标、高亮、图形、对齐表格与有序列表", () => {
    const body = render("叶脉笔记.md");
    for (const selector of ["h1", "h2", "blockquote.callout", "math", 'math[display="block"]', "sub", "mark", "p code", "a[href]", "pre code.language-mermaid", "table", "ol > li sup"]) {
      expect(body.querySelector(selector), selector).not.toBeNull();
    }
    expect([...body.querySelectorAll("th")].map(cell => cell.getAttribute("align") ?? cell.style.textAlign)).toEqual(["left", "left", "right"]);
  });
});
