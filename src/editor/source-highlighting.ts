import { Prec, type Extension } from "@codemirror/state";
import { Decoration, EditorView, ViewPlugin, type DecorationSet, type ViewUpdate } from "@codemirror/view";
import { defaultHighlightStyle, HighlightStyle, StreamLanguage, syntaxHighlighting, syntaxTree, type Language } from "@codemirror/language";
import { markdown } from "@codemirror/lang-markdown";
import { GFM } from "@lezer/markdown";
import { tags } from "@lezer/highlight";
import { javascript, javascriptLanguage, typescriptLanguage } from "@codemirror/lang-javascript";
import { jsonLanguage } from "@codemirror/lang-json";
import { pythonLanguage } from "@codemirror/lang-python";
import { goLanguage } from "@codemirror/lang-go";
import { rustLanguage } from "@codemirror/lang-rust";
import { sql, StandardSQL } from "@codemirror/lang-sql";
import { htmlLanguage } from "@codemirror/lang-html";
import { cssLanguage } from "@codemirror/lang-css";
import { shell } from "@codemirror/legacy-modes/mode/shell";

const languages: Readonly<Record<string, Language>> = {
  javascript: javascriptLanguage, js: javascriptLanguage, jsx: javascript({ jsx: true }).language,
  typescript: typescriptLanguage, ts: typescriptLanguage, tsx: javascript({ typescript: true, jsx: true }).language,
  json: jsonLanguage, python: pythonLanguage, py: pythonLanguage,
  go: goLanguage, golang: goLanguage, rust: rustLanguage, rs: rustLanguage,
  shell: StreamLanguage.define(shell), sql: sql({ dialect: StandardSQL }).language,
  html: htmlLanguage, css: cssLanguage,
};

/** 围栏语言只按首个标识匹配，允许额外属性；未知语言保持普通代码文本。 */
export function sourceCodeLanguage(info: string): Language | null {
  const name = info.trim().split(/\s+/)[0]?.toLowerCase() ?? "";
  if (["sh", "bash", "zsh", "shellscript"].includes(name)) return languages.shell!;
  return Object.hasOwn(languages, name) ? languages[name]! : null;
}

// 所有颜色支持主题覆盖；回退到现有主题变量，无需额外 CSS 即可显示。
export const sourceHighlightStyle = HighlightStyle.define([
  { tag: [tags.heading, tags.heading1, tags.heading2, tags.heading3], color: "var(--syntax-heading, var(--accent))", fontWeight: "600" },
  { tag: tags.strong, fontWeight: "700" },
  { tag: tags.emphasis, fontStyle: "italic" },
  { tag: tags.strikethrough, textDecoration: "line-through" },
  { tag: [tags.link, tags.url], color: "var(--syntax-link, var(--accent))", textDecoration: "underline" },
  { tag: [tags.keyword, tags.controlKeyword, tags.operatorKeyword], color: "var(--syntax-keyword, var(--accent))" },
  { tag: [tags.string, tags.special(tags.string), tags.regexp], color: "var(--syntax-string, #60825b)" },
  { tag: [tags.number, tags.bool, tags.null], color: "var(--syntax-number, #a47542)" },
  { tag: [tags.comment, tags.meta], color: "var(--syntax-comment, var(--muted, #888))", fontStyle: "italic" },
  { tag: [tags.typeName, tags.className, tags.tagName], color: "var(--syntax-type, #7b76b0)" },
  { tag: [tags.propertyName, tags.attributeName], color: "var(--syntax-property, #51818f)" },
  { tag: [tags.function(tags.variableName), tags.function(tags.propertyName)], color: "var(--syntax-function, #51818f)" },
  { tag: [tags.operator, tags.punctuation, tags.processingInstruction], color: "var(--syntax-punctuation, var(--muted, #888))" },
  { tag: tags.monospace, color: "var(--syntax-code, var(--accent))", fontFamily: "var(--mono, monospace)" },
  { tag: tags.attributeValue, color: "var(--syntax-code, var(--accent))" },
  { tag: tags.invalid, textDecoration: "underline wavy", textDecorationColor: "var(--syntax-error, #b34b4b)" },
]);

const sourceCodeLine = Decoration.line({ class: "md-source-code" });
function codeLines(view: EditorView): DecorationSet {
  const ranges: ReturnType<Decoration["range"]>[] = [];
  let lastLine = 0;
  for (const { from, to } of view.visibleRanges) {
    syntaxTree(view.state).iterate({ from, to, enter(node) {
      if (node.name !== "FencedCode" && node.name !== "CodeBlock") return;
      const first = view.state.doc.lineAt(Math.max(node.from, from)).number;
      const last = view.state.doc.lineAt(Math.min(node.to, to)).number;
      for (let number = Math.max(first, lastLine + 1); number <= last; number++) ranges.push(sourceCodeLine.range(view.state.doc.line(number).from));
      lastLine = Math.max(lastLine, last);
      return false;
    } });
  }
  return Decoration.set(ranges, true);
}
/** 给围栏和缩进代码的每一行加等宽样式；嵌套语言解析不改变行的归属。 */
const sourceCodeLines = ViewPlugin.fromClass(class {
  decorations: DecorationSet;
  constructor(view: EditorView) { this.decorations = codeLines(view); }
  update(update: ViewUpdate) {
    if (update.docChanged || update.viewportChanged || syntaxTree(update.startState) !== syntaxTree(update.state)) this.decorations = codeLines(update.view);
  }
}, { decorations: value => value.decorations });

/** 挂在源码模式 compartment 中；优先使用带嵌套语言解析的 Markdown，移除后恢复原位模式。 */
export function sourceHighlighting(): Extension {
  return [
    Prec.high(markdown({ extensions: [GFM], codeLanguages: sourceCodeLanguage })),
    syntaxHighlighting(sourceHighlightStyle),
    syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
    sourceCodeLines,
  ];
}
