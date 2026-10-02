import { expect, test } from "bun:test";
import { EditorState, Text } from "@codemirror/state";
import { markdown } from "@codemirror/lang-markdown";
import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { analyzeDocument } from "../../src/editor/document-stats";

function state(doc: string) {
  return EditorState.create({ doc, extensions: [markdown()] });
}

test("混排英文汉字不会漏词，emoji 不计数，组合字符保持在同一单词", () => {
  expect(analyzeDocument(state("ABC中文def 👩‍💻 😀 café café 123\n中文ABC"))).toMatchObject({ wordCount: 10, readMinutes: 1 });
  expect(analyzeDocument(state("😀 👩‍💻 🏳️‍🌈 1️⃣ 2⃣ #️⃣ ! --"))).toMatchObject({ wordCount: 0, readMinutes: 1 });
  expect(analyzeDocument(state("𠀀甲ABC乙def")).wordCount).toBe(5);
});

test("阅读时长按每分钟 250 字向上取整", () => {
  expect(analyzeDocument(state("字".repeat(251))).readMinutes).toBe(2);
});

test("语法树识别 ATX、Setext 与嵌套标题并清理简单 Markdown", () => {
  const source = "  # **粗体** [链接](https://example.com) ##\n\nSetext *标题*\n===\n\n二级 `code`\n---\n\n> ### 引用标题\n\n- #### 列表标题\n";
  const docState = state(source);
  const headings = analyzeDocument(docState).headings;
  expect(headings.map(({ level, title }) => ({ level, title }))).toEqual([
    { level: 1, title: "粗体 链接" },
    { level: 1, title: "Setext 标题" },
    { level: 2, title: "二级 code" },
    { level: 3, title: "引用标题" },
    { level: 4, title: "列表标题" },
  ]);
  expect(docState.doc.sliceString(headings[0]!.from, headings[0]!.to)).toBe("# **粗体** [链接](https://example.com) ##");
  expect(docState.doc.sliceString(headings[1]!.from, headings[1]!.to)).toBe("Setext *标题*\n===");
});

test("忽略围栏、缩进代码和引用定义中的伪标题", () => {
  const docState = state("````markdown\n# fake\n```\n## still fake\n````\n\n~~~\n# fake too\n~~~\n\n    # indented\n\n[ref]: https://example.com\n  \"# reference title\"\n\n# real");
  expect(analyzeDocument(docState).headings.map(heading => heading.title)).toEqual(["real"]);
});

test("忽略 YAML frontmatter，同时保留后续 Setext 标题", () => {
  for (const ending of ["---", "..."]) {
    const docState = state(`---\ntitle: metadata\n# metadata heading\n${ending}\n\nReal title\n---\n\n# Body`);
    expect(analyzeDocument(docState).headings.map(heading => heading.title)).toEqual(["Real title", "Body"]);
  }
  // 没有结束符时开头的 --- 仍是 Markdown 分隔线。
  expect(analyzeDocument(state("---\n# Body")).headings.map(heading => heading.title)).toEqual(["Body"]);
});

test("选择变化复用缓存，正文编辑刷新字数与标题", () => {
  const before = state("# Hello\n\nABC中文def");
  const stats = analyzeDocument(before);
  expect(analyzeDocument(before.update({ selection: { anchor: 3 } }).state)).toBe(stats);
  const after = before.update({ changes: { from: 2, to: 7, insert: "World 中文" } }).state;
  expect(analyzeDocument(after).wordCount).toBe(stats.wordCount + 2);
  expect(analyzeDocument(after).headings[0]!.title).toBe("World 中文");
});

test("字数缓存复用不可变文本子树，无需把文档转换成完整字符串", () => {
  const before = state("ABC中文def\n".repeat(5000));
  expect(analyzeDocument(before).wordCount).toBe(20000);
  const original = Text.prototype.toString;
  Text.prototype.toString = () => { throw new Error("禁止全文字符串分配"); };
  try {
    const after = before.update({ changes: { from: 0, insert: "新" } }).state;
    expect(analyzeDocument(after).wordCount).toBe(20001);
  } finally { Text.prototype.toString = original; }
});

test("大文档只读取已有语法树，后台解析推进后同一文档可刷新大纲", () => {
  const before = state("paragraph\n\n".repeat(2000) + "# End");
  const initialTree = syntaxTree(before);
  const initial = analyzeDocument(before);
  expect(syntaxTree(before)).toBe(initialTree);
  expect(initial.headings).toEqual([]);
  ensureSyntaxTree(before, before.doc.length, 1000);
  const progressed = before.update({}).state;
  expect(progressed.doc).toBe(before.doc);
  expect(analyzeDocument(progressed).headings.map(heading => heading.title)).toEqual(["End"]);
});
