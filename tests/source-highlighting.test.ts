import { expect, test } from "bun:test";
import { Compartment, EditorState } from "@codemirror/state";
import { ensureSyntaxTree, highlightingFor, syntaxTree } from "@codemirror/language";
import { markdown } from "@codemirror/lang-markdown";
import { highlightTree } from "@lezer/highlight";
import { sourceCodeLanguage, sourceHighlighting, sourceHighlightStyle } from "../src/source-highlighting";

function parsed(content: string) {
  // 模拟主界面原有 Markdown 扩展，验证源码 compartment 能优先接管围栏解析。
  const state = EditorState.create({ doc: content, extensions: [markdown(), sourceHighlighting()] });
  const tree = ensureSyntaxTree(state, state.doc.length, 1000);
  expect(tree).not.toBeNull();
  const spans: string[] = [];
  highlightTree(tree!, sourceHighlightStyle, (from, to) => { spans.push(state.sliceDoc(from, to)); });
  return { state, tree: tree!, spans };
}

test("Markdown 标题、强调、链接和 GFM 删除线得到真实语法高亮", () => {
  const { tree, spans } = parsed("# 标题\n\n**粗体** *斜体* ~~删除~~ [链接](https://example.com)\n");
  expect(tree.toString()).toContain("Strikethrough");
  for (const text of ["标题", "粗体", "斜体", "删除", "链接"]) {
    expect(spans.some(span => span.includes(text))).toBe(true);
  }
});

const examples: [string, string, string][] = [
  ["javascript", "const answer = 42; // 注释", "const"],
  ["typescript", "const answer: number = 42;", "number"],
  ["json", '{"answer": true}', "true"],
  ["python", "def answer():\n    return 42", "return"],
  ["go", "package main\nfunc answer() int { return 42 }", "return"],
  ["rust", "fn main() { let answer = 42; }", "let"],
  ["shell", 'if true; then echo "你好"; fi', "if"],
  ["sql", "SELECT name FROM notes WHERE id = 42;", "SELECT"],
  ["html", '<div class="note">正文</div>', "div"],
  ["css", ".note { color: red; }", "color"],
];
for (const [language, code, token] of examples) {
  test(`${language} 围栏使用嵌套语言并高亮实际词元`, () => {
    const { state, spans } = parsed(`\`\`\`${language}\n${code}\n\`\`\`\n`);
    const position = state.doc.toString().indexOf(token, language.length + 4);
    expect(sourceCodeLanguage(language)!.isActiveAt(state, position + 1)).toBe(true);
    expect(spans.some(span => span.includes(token))).toBe(true);
  });
}

test("语言别名和围栏属性可识别，未知语言安全回退", () => {
  expect(sourceCodeLanguage(" TS filename=note.ts")).toBe(sourceCodeLanguage("typescript"));
  expect(sourceCodeLanguage("bash")).toBe(sourceCodeLanguage("shell"));
  expect(sourceCodeLanguage("toString")).toBeNull();
  expect(sourceCodeLanguage("unknown")).toBeNull();
  const { tree } = parsed("```unknown\nconst answer = 42;\n```\n");
  expect(tree.toString()).toContain("CodeText");
});

test("移除源码 compartment 后嵌套解析与源码高亮一起移除", () => {
  const compartment = new Compartment();
  let state = EditorState.create({ doc: "```python\ndef answer(): return 42\n```", extensions: [markdown(), compartment.of(sourceHighlighting())] });
  ensureSyntaxTree(state, state.doc.length, 1000);
  const position = state.doc.toString().indexOf("return") + 1;
  expect(sourceCodeLanguage("python")!.isActiveAt(state, position)).toBe(true);
  state = state.update({ effects: compartment.reconfigure([]) }).state;
  expect(sourceCodeLanguage("python")!.isActiveAt(state, position)).toBe(false);
  expect(syntaxTree(state).toString()).toContain("CodeText");
  expect(highlightingFor(state, [])).toBeNull();
});
