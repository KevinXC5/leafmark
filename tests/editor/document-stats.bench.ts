import { EditorState } from "@codemirror/state";
import { markdown } from "@codemirror/lang-markdown";
import { syntaxTree } from "@codemirror/language";
import { analyzeDocument } from "../../src/editor/document-stats";

// 运行：bun tests/editor/document-stats.bench.ts。规模按 UTF-8 字节计算。
const paragraph = "ABC中文def a short paragraph about document performance. 😀\n\n";
const encoder = new TextEncoder();
function measure(fn: () => void): number {
  const start = performance.now();
  fn();
  return performance.now() - start;
}
function round(value: number): number { return Number(value.toFixed(4)); }

for (const bytes of [1024 * 1024, 16 * 1024 * 1024]) {
  const prefix = "# Benchmark\n\n";
  const copies = Math.floor((bytes - encoder.encode(prefix).length) / encoder.encode(paragraph).length);
  const source = prefix + paragraph.repeat(copies);
  let state!: EditorState;
  const createMs = measure(() => { state = EditorState.create({ doc: source, extensions: [markdown()] }); });
  const coldMs = measure(() => { analyzeDocument(state); });
  const cachedMs = measure(() => { for (let i = 0; i < 1000; i++) analyzeDocument(state); }) / 1000;
  const edits: number[] = [];
  for (let index = 0; index < 20; index++) {
    state = state.update({ changes: { from: Math.floor(state.doc.length / 2), insert: "新" } }).state;
    edits.push(measure(() => { analyzeDocument(state); }));
  }
  edits.sort((a, b) => a - b);
  console.log(JSON.stringify({
    utf8Bytes: encoder.encode(source).length,
    utf16Length: source.length,
    createMs: round(createMs),
    coldMs: round(coldMs),
    cachedMs: round(cachedMs),
    editMedianMs: round(edits[Math.floor(edits.length / 2)]!),
    editMaxMs: round(edits.at(-1)!),
    parsedUntil: syntaxTree(state).length,
    wordCount: analyzeDocument(state).wordCount,
  }));
}
