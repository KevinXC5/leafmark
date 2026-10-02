import { describe, expect, test } from "bun:test";
import { EditorState } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";
import { markdown } from "@codemirror/lang-markdown";
import { GFM } from "@lezer/markdown";
import { buildLiveMarkdownDecorations, isPreviewImageUrl } from "../../../src/editor/live/live-markdown";

function decorations(source: string, focused = false, showActiveSyntax = true) {
  const state = EditorState.create({ doc: source, extensions: [markdown({ extensions: [GFM] })] });
  const view = { state, hasFocus: focused, visibleRanges: [{ from: 0, to: source.length }] } as unknown as EditorView;
  const result: { from: number; to: number; className?: string; widget?: string; replaced: boolean }[] = [];
  buildLiveMarkdownDecorations(view, { showActiveSyntax }).between(0, source.length, (from, to, value) => {
    result.push({ from, to, className: value.spec.class, widget: value.spec.widget?.constructor.name, replaced: value.spec.isBlock === false || value.spec.widget !== undefined });
  });
  return result;
}

describe("实时 Markdown 装饰", () => {
  test("多 backtick 使用 CodeMark 的真实范围", () => {
    const result = decorations("``a`b``");
    expect(result.find(range => range.className === "md-inline-code")).toMatchObject({ from: 2, to: 5 });
    expect(result.filter(range => range.from !== range.to && !range.className).map(({ from, to }) => [from, to])).toEqual([[0, 2], [5, 7]]);
  });
  test("active line 默认显示源码，选项可强制预览", () => {
    expect(decorations("**bold**", true).filter(range => !range.className)).toHaveLength(0);
    expect(decorations("**bold**", true, false).filter(range => !range.className)).toHaveLength(2);
  });
  test("Setext 标题和分隔线", () => {
    const result = decorations("Title\n===\n\n---");
    expect(result.filter(range => range.className === "md-h1")).toHaveLength(2);
    expect(result.some(range => range.className === "lm-setext-delimiter")).toBe(true);
    expect(result.some(range => range.widget === "Rule")).toBe(true);
  });
  test("链接和 Autolink 显示文本样式", () => {
    const source = "[label](https://a) <https://b>";
    const result = decorations(source);
    expect(result.filter(range => range.className === "lm-markdown-link").map(({ from, to }) => source.slice(from, to))).toEqual(["label", "https://b"]);
  });
  test("安全图片整体替换，相对及危险资源保留源码", () => {
    const result = decorations("![**alt**](https://example.com/a.png) ![local](./a.png) ![bad](javascript:x)");
    expect(result.filter(range => range.widget === "ImagePreview")).toHaveLength(1);
    expect(result.some(range => range.className === "md-strong")).toBe(false);
    for (const url of ["./a.png", "//host/a.png", "javascript:x", "data:image/svg+xml;base64,eA==", "https://a\\b", "javascript&#58;x"]) expect(isPreviewImageUrl(url)).toBe(false);
    for (const url of ["https://example.com/a.png", "http://example.com/a.png", "data:image/png;base64,eA=="]) expect(isPreviewImageUrl(url)).toBe(true);
  });
  test("相对图片解析缓存、异步通知去重且不修改源文", async () => {
    const source = "![cached](./cached.png) ![missing](./missing.png) ![again](./missing.png) ![bad](javascript:x)";
    const state = EditorState.create({ doc: source, extensions: [markdown({ extensions: [GFM] })] });
    const view = { state, hasFocus: false, visibleRanges: [{ from: 0, to: source.length }] } as unknown as EditorView;
    const needed: string[] = [];
    const resolved: string[] = [];
    const preview = buildLiveMarkdownDecorations(view, {
      resolveImage: url => { resolved.push(url); return url === "./cached.png" ? "data:image/png;base64,eA==" : undefined; },
      onImageNeeded: url => needed.push(url),
    });
    const urls: string[] = [];
    preview.between(0, source.length, (_, __, decoration) => { if (decoration.spec.widget) urls.push(decoration.spec.widget.url); });
    expect(urls).toEqual(["data:image/png;base64,eA=="]);
    expect(resolved).not.toContain("javascript:x");
    expect(needed).toEqual([]);
    await Promise.resolve();
    expect(needed).toEqual(["./missing.png"]);
    expect(state.doc.toString()).toBe(source);
  });
  test("wiki 图片复用安全资源缓存和尺寸，缺失资源通知去重", async () => {
    const source = "![[assets/配图.svg|480]] ![[missing.png]] ![[missing.png]]";
    const state = EditorState.create({ doc: source, extensions: [markdown({ extensions: [GFM] })] });
    const view = { state, hasFocus: false, visibleRanges: [{ from: 0, to: source.length }] } as unknown as EditorView;
    const needed: string[] = [];
    const widgets: { constructor: { name: string }; width?: number }[] = [];
    buildLiveMarkdownDecorations(view, {
      resolveImage: url => url === "assets/配图.svg" ? "data:image/png;base64,eA==" : undefined,
      onImageNeeded: url => needed.push(url),
    }).between(0, source.length, (_, __, value) => { if (value.spec.widget) widgets.push(value.spec.widget); });
    expect(widgets.map(widget => widget.constructor.name)).toEqual(["ImagePreview", "WikiPreview", "WikiPreview"]);
    expect(widgets[0]?.width).toBe(480);
    await Promise.resolve();
    expect(needed).toEqual(["missing.png"]);
    expect(state.doc.toString()).toBe(source);
  });
  test("解析后的不安全资源不得成为图片 widget", () => {
    const source = "![local](./a.png)";
    const state = EditorState.create({ doc: source, extensions: [markdown()] });
    const view = { state, hasFocus: false, visibleRanges: [{ from: 0, to: source.length }] } as unknown as EditorView;
    expect(buildLiveMarkdownDecorations(view, { resolveImage: () => "javascript:alert(1)" }).size).toBe(0);
  });
  test("普通列表有 bullet，任务列表没有重复 bullet 或重叠替换", () => {
    const result = decorations("- item\n  - nested\n- [ ] task\n> - [x] quoted");
    expect(result.filter(range => range.widget === "Bullet")).toHaveLength(2);
    expect(result.filter(range => range.widget === "TaskBox")).toHaveLength(2);
    const replacements = result.filter(range => !range.className);
    for (let i = 0; i < replacements.length; i++) {
      for (let j = i + 1; j < replacements.length; j++) {
        const a = replacements[i]!;
        const b = replacements[j]!;
        expect(a.from < b.to && a.to > b.from).toBe(false);
      }
    }
  });
  test("真实笔记的 wiki 链接完整替换，当前行保留全部语法", () => {
    const source = "[[文件|别名]] · [[文件#标题|别名]] · [[#标题]] · [[#^块]]";
    const result = decorations(source);
    expect(result.filter(range => range.widget === "WikiPreview").map(range => source.slice(range.from, range.to))).toEqual(["[[文件|别名]]", "[[文件#标题|别名]]", "[[#标题]]", "[[#^块]]"]);
    expect(result.filter(range => !range.className)).toHaveLength(4);
    expect(decorations(source, true).filter(range => !range.className)).toHaveLength(0);
    expect(decorations(source, true, false).filter(range => range.widget === "WikiPreview")).toHaveLength(4);
  });
  test("嵌入显示完整占位，高亮隐藏分隔符且保留正文强调", () => {
    const source = "![[不存在的音频.mp3]] ![[不存在的文档.pdf#page=1]] ==**高亮**==";
    const result = decorations(source);
    expect(result.filter(range => range.widget === "WikiPreview")).toHaveLength(2);
    expect(result.find(range => range.className === "lm-highlight")).toBeDefined();
    expect(result.find(range => range.className === "md-strong")).toBeDefined();
    expect(decorations(source, true).filter(range => !range.className)).toHaveLength(0);
  });
  test("wiki 和高亮不进入代码、外部链接或转义文本", () => {
    const source = "`[[文件]] ==高亮==`\n\n```md\n[[文件]] ==高亮==\n```\n\n[==外链==](https://example.com/[[文件]]) \\[[转义]] \\==转义==";
    const result = decorations(source);
    expect(result.some(range => range.widget === "WikiPreview" || range.className === "lm-highlight")).toBe(false);
  });
  test("Callout 默认标题、别名和嵌套引用具有语义装饰", () => {
    const result = decorations("> [!faq]- 默认折叠\n> 内容\n>\n> > [!tip]\n> > 提示");
    expect(result.some(range => range.className === "lm-callout lm-callout-question")).toBe(true);
    expect(result.some(range => range.className === "lm-callout lm-callout-tip")).toBe(true);
    expect(result.some(range => range.widget === "CalloutTitle")).toBe(true);
    expect(decorations("> [!note]", true).some(range => range.widget === "CalloutTitle")).toBe(false);
  });
  test("代码块内部不生成标题、图片或列表预览", () => {
    const result = decorations("```md\n- [ ] ![alt](https://x)\n```");
    expect(result.some(range => range.widget)).toBe(false);
  });
});
