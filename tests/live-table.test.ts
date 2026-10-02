import { expect, test } from "bun:test";
import { EditorState } from "@codemirror/state";
import { buildLiveTableDecorations, liveTable, MAX_TABLE_PREVIEW_CHARACTERS } from "../src/live-table";

const source = "| A | B |\n| --- | ---: |\n| x | y |";
function count(state: EditorState, focused: boolean, showActiveSyntax = true) {
  let count = 0;
  buildLiveTableDecorations(state, focused, { showActiveSyntax }).between(0, state.doc.length, (from, to, decoration) => {
    expect(decoration.spec.block).toBe(true);
    expect(from).toBe(0);
    expect(to).toBe(source.length);
    count++;
  });
  return count;
}
test("表格使用整个语法范围的 StateField block decoration", () => {
  const state = EditorState.create({ doc: source, extensions: [liveTable()] });
  expect(count(state, false)).toBe(1);
  expect(count(state, true)).toBe(0);
  expect(count(state, true, false)).toBe(1);
});
test("选区离开表格时聚焦也显示表格", () => {
  const state = EditorState.create({ doc: source + "\n\nafter", selection: { anchor: source.length + 2 } });
  expect(count(state, true)).toBe(1);
});
test("不跨行替换引用、列表及代码块中的表格", () => {
  for (const doc of ["> | A |\n> | --- |\n> | x |", "- | A |\n  | --- |\n  | x |", "```\n" + source + "\n```", "普通文本"]) {
    const state = EditorState.create({ doc });
    expect(buildLiveTableDecorations(state, false).size).toBe(0);
  }
});

test("超大文档不进行表格整块替换", () => {
  const state = EditorState.create({ doc: source + "\n\n" + "a".repeat(MAX_TABLE_PREVIEW_CHARACTERS) });
  expect(buildLiveTableDecorations(state, false).size).toBe(0);
});
