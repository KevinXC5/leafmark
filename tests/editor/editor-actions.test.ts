import { describe, expect, test } from "bun:test";
import { EditorSelection, EditorState } from "@codemirror/state";
import { history, redo, undo } from "@codemirror/commands";
import { applyFormat, createFormatTransaction, type FormatAction } from "../../src/editor/editor-actions";
import type { EditorView } from "@codemirror/view";

function format(doc: string, action: FormatAction, anchor = 0, head = doc.length) {
  const state = EditorState.create({ doc, selection: { anchor, head } });
  return state.update(createFormatTransaction(state, action)!).state;
}

describe("行内格式", () => {
  test.each([["strong", "**文字**"], ["em", "*文字*"], ["strike", "~~文字~~"], ["code", "`文字`"]] as const)("%s 包裹、选区和切换", (action, expected) => {
    const state = format("文字", action);
    expect(state.doc.toString()).toBe(expected);
    expect(state.sliceDoc(state.selection.main.from, state.selection.main.to)).toBe("文字");
    expect(state.update(createFormatTransaction(state, action)!).state.doc.toString()).toBe("文字");
  });
  test("反向选区保持方向，空光标留在标记内部", () => {
    expect(format("文本", "strong", 2, 0).selection.main.anchor).toBeGreaterThan(format("文本", "strong", 2, 0).selection.main.head);
    const empty = format("", "em");
    expect(empty.doc.toString()).toBe("**");
    expect(empty.selection.main.head).toBe(1);
  });
  test("行内代码选择不会与正文的反引号冲突", () => {
    const code = format("a`b", "code");
    expect(code.doc.toString()).toBe("``a`b``");
    expect(code.update(createFormatTransaction(code, "code")!).state.doc.toString()).toBe("a`b");
    const padded = format("`边界", "code");
    expect(padded.update(createFormatTransaction(padded, "code")!).state.doc.toString()).toBe("`边界");
    expect(format("`边界`", "code").doc.toString()).toBe("边界");
    expect(format("`边界", "code").doc.toString()).toBe("`` `边界 ``");
  });
  test("多行逐行处理，保留空段", () => {
    expect(format("一\n\n二", "strong").doc.toString()).toBe("**一**\n\n**二**");
  });
  test("链接、图片参数保持 Markdown 结构", () => {
    expect(format("文本", { type: "link", url: "https://example.com/a(b)" }).doc.toString()).toBe("[文本](<https://example.com/a(b)>)");
    expect(format("", "image").doc.toString()).toBe("![图片描述](<https://>)");
    expect(format("[文本]", { type: "link", url: "https://example.com/a>b" }).doc.toString()).toContain("%3E");
  });
});

describe("块格式", () => {
  test.each([1, 2, 3, 4, 5, 6] as const)("标题级别 %s 替换与移除", level => {
    const state = format("## 一\n二", { type: "heading", level });
    expect(state.doc.toString()).toBe(`${"#".repeat(level)} 一\n${"#".repeat(level)} 二`);
    expect(state.update(createFormatTransaction(state, { type: "heading", level })!).state.doc.toString()).toBe("一\n二");
  });
  test.each([1, 2, 3, 4, 5, 6] as const)("兼容主界面 h%s 动作", level => {
    expect(format("标题", `h${level}`).doc.toString()).toBe(`${"#".repeat(level)} 标题`);
  });
  test("多行列表转换、缩进及任务状态移除", () => {
    expect(format("  - [x] 一\n  - 二", "ordered").doc.toString()).toBe("  1. 一\n  2. 二");
    expect(format("- [x] 一\n- [ ] 二", "task").doc.toString()).toBe("一\n二");
    expect(format("一\n二", "bullet").doc.toString()).toBe("- 一\n- 二");
    expect(format("一\n二", "quote").doc.toString()).toBe("> 一\n> 二");
  });
  test("结束在下一行开头不修改下一行", () => {
    expect(format("一\n二", "quote", 0, 2).doc.toString()).toBe("> 一\n二");
  });
  test("围栏避免与正文冲突并可整体切换", () => {
    const empty = format("", "codeblock");
    expect(empty.doc.toString()).toBe("```\n\n```");
    expect(empty.selection.main.head).toBe(4);
    const state = format("const a = 1;", { type: "codeblock", language: "javascript" });
    expect(state.doc.toString()).toBe("```javascript\nconst a = 1;\n```");
    expect(format(state.doc.toString(), "codeblock").doc.toString()).toBe("const a = 1;");
    expect(format("```\nx\n```", { type: "codeblock", language: "ts" }).doc.toString()).toBe("x");
    expect(format("a ``` b", "codeblock").doc.toString()).toBe("````\na ``` b\n````");
  });
  test("表格和分隔线独立于正文", () => {
    expect(format("正文", "hr").doc.toString()).toBe("正文\n\n---");
    expect(format("前\n\n后", "table", 2, 2).doc.toString()).toContain("前\n\n| 列 1 |");
    expect(format("前\n后", "hr", 2, 2).doc.toString()).toBe("前\n\n后\n\n---");
  });
});

test("多光标和共享行仅应用一次", () => {
  const state = EditorState.create({ doc: "一 二\n三", selection: EditorSelection.create([EditorSelection.cursor(0), EditorSelection.cursor(2), EditorSelection.cursor(4)]), extensions: EditorState.allowMultipleSelections.of(true) });
  const next = state.update(createFormatTransaction(state, "ordered")!).state;
  expect(next.doc.toString()).toBe("1. 一 二\n2. 三");
  const inline = state.update(createFormatTransaction(state, "strong")!).state;
  expect(inline.selection.ranges.length).toBe(3);
  expect(inline.doc.toString()).toBe("****一 ****二\n****三");
});

test("一次操作单独撤销，并保留之前和之后的输入", () => {
  let state = EditorState.create({ doc: "", extensions: [history()] });
  const dispatch = (transaction: Parameters<typeof state.update>[0] | { state: EditorState }) => {
    state = "state" in transaction ? transaction.state : state.update(transaction).state;
  };
  state = state.update({ changes: { from: 0, insert: "一\n二" }, selection: { anchor: 0, head: 3 }, userEvent: "input.type" }).state;
  state = state.update(createFormatTransaction(state, "quote")!).state;
  state = state.update({ changes: { from: state.doc.length, insert: "后" }, userEvent: "input.type" }).state;
  const target = { get state() { return state; }, dispatch };
  expect(undo(target)).toBe(true);
  expect(state.doc.toString()).toBe("> 一\n> 二");
  expect(undo(target)).toBe(true);
  expect(state.doc.toString()).toBe("一\n二");
  expect(undo(target)).toBe(true);
  expect(state.doc.toString()).toBe("");
  expect(redo(target)).toBe(true);
});

test("applyFormat 分发事务、恢复焦点并尊重只读状态", () => {
  let state = EditorState.create({ doc: "文本", selection: { anchor: 0, head: 2 } });
  let focused = false;
  const view = { get state() { return state; }, dispatch(spec: Parameters<typeof state.update>[0]) { state = state.update(spec).state; }, focus() { focused = true; } } as unknown as EditorView;
  expect(applyFormat(view, "strong")).toBe(true);
  expect(state.doc.toString()).toBe("**文本**");
  expect(focused).toBe(true);
  const readonly = EditorState.create({ extensions: EditorState.readOnly.of(true) });
  expect(createFormatTransaction(readonly, "strong")).toBeNull();
});
