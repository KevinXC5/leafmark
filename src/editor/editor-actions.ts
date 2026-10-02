import { EditorSelection, type EditorState, type TransactionSpec } from "@codemirror/state";
import { isolateHistory } from "@codemirror/commands";
import type { EditorView } from "@codemirror/view";

export type HeadingLevel = 1 | 2 | 3 | 4 | 5 | 6;
export type FormatAction =
  | "strong" | "bold" | "em" | "italic" | "strike" | "code" | "inline-code"
  | `h${HeadingLevel}` | `heading${HeadingLevel}` | "heading" | "quote" | "bullet" | "ordered" | "task"
  | "codeblock" | "link" | "image" | "table" | "hr"
  | { type: "heading"; level: HeadingLevel }
  | { type: "link" | "image"; url?: string; title?: string }
  | { type: "codeblock"; language?: string };

const longestTicks = (text: string) => Math.max(0, ...(text.match(/`+/g) ?? []).map(run => run.length));

/** 构造纯状态事务，便于测试，也可供非工具栏入口复用。 */
export function createFormatTransaction(state: EditorState, action: FormatAction): TransactionSpec | null {
  if (state.readOnly) return null;
  const type = typeof action === "string" ? action : action.type;
  const heading = type === "heading" ? (typeof action === "object" && action.type === "heading" ? action.level : 1)
    : /^(?:heading|h)[1-6]$/.test(type) ? Number(type.at(-1)) : 0;
  let spec: TransactionSpec;

  if (heading || ["quote", "bullet", "ordered", "task", "codeblock", "table", "hr"].includes(type)) {
    // 选区恰好结束在下一行开头时，不把该行算入操作；多个光标共享的行只修改一次。
    const numbers = new Set<number>();
    for (const range of state.selection.ranges) {
      const first = state.doc.lineAt(range.from).number;
      const last = state.doc.lineAt(range.to > range.from && state.doc.lineAt(range.to).from === range.to ? range.to - 1 : range.to).number;
      for (let n = first; n <= last; n++) numbers.add(n);
    }
    const lines = [...numbers].sort((a, b) => a - b).map(n => state.doc.line(n));
    const changes: { from: number; to: number; insert: string }[] = [];
    const codeOffsets = new Map<number, number>();
    if (["codeblock", "table", "hr"].includes(type)) {
      const groups: typeof lines[] = [];
      for (const line of lines) {
        const group = groups.at(-1);
        if (group && group.at(-1)!.number + 1 === line.number) group.push(line);
        else groups.push([line]);
      }
      for (const group of groups) {
        const from = group[0]!.from, to = group.at(-1)!.to;
        const text = state.sliceDoc(from, to);
        let insert: string;
        if (type === "codeblock") {
          const language = typeof action === "object" && action.type === "codeblock" ? (action.language ?? "").replace(/[^\w+-]/g, "") : "";
          const fence = "`".repeat(Math.max(3, longestTicks(text) + 1));
          const wrapped = text.match(/^(`{3,}|~{3,})[^\n]*\n([\s\S]*)\n\1[ \t]*$/);
          insert = wrapped ? wrapped[2]! : `${fence}${language}\n${text}\n${fence}`;
          if (!wrapped && group.length === 1 && !text) codeOffsets.set(from, fence.length + language.length + 1);
        } else {
          const block = type === "hr" ? "---" : "| 列 1 | 列 2 |\n| --- | --- |\n| 内容 | 内容 |";
          // 保留选中正文，在其后插入独立块；空行则直接成为插入点。
          insert = (text.trim() ? text + "\n\n" : "") + block;
        }
        // 块前后留空行，避免分隔线成为 Setext 标题、表格或围栏粘连正文。
        if (from > 0 && state.doc.line(group[0]!.number - 1).text.trim()) {
          insert = "\n" + insert;
          if (codeOffsets.has(from)) codeOffsets.set(from, codeOffsets.get(from)! + 1);
        }
        if (to < state.doc.length && state.doc.line(group.at(-1)!.number + 1).text.trim()) insert += "\n";
        changes.push({ from, to, insert });
      }
    } else {
      const marker = heading ? "#".repeat(heading) + " " : type === "quote" ? "> " : type === "task" ? "- [ ] " : "- ";
      const pattern = heading ? /^(#{1,6})[ \t]+/ : type === "quote" ? /^>[ \t]?/ : type === "task" ? /^(?:[-+*]|\d+[.)])[ \t]+\[[ xX]\][ \t]+/ : type === "ordered" ? /^\d+[.)][ \t]+/ : /^[-+*][ \t]+(?!\[[ xX]\][ \t])/;
      const parts = lines.map(line => {
        const indent = line.text.match(/^[ \t]*/)![0];
        const body = line.text.slice(indent.length);
        const match = body.match(pattern);
        return { line, indent, body, match };
      });
      const remove = parts.every(({ body, match }) => !!match && (!heading || body.startsWith(marker)));
      let ordinal = 0, previous = -1;
      for (const { line, indent, body, match } of parts) {
        ordinal = previous + 1 === line.number ? ordinal + 1 : 1;
        previous = line.number;
        // 列表类型切换时去掉旧标记，保留缩进、正文及其行内格式。
        const old = match?.[0] ?? (!heading && type !== "quote" ? body.match(/^(?:[-+*]|\d+[.)])[ \t]+(?:\[[ xX]\][ \t]+)?/)?.[0] : undefined) ?? "";
        const prefix = remove ? "" : type === "ordered" ? `${ordinal}. ` : marker;
        changes.push({ from: line.from + indent.length, to: line.from + indent.length + old.length, insert: prefix });
      }
    }
    const changeSet = state.changes(changes);
    const mapped = state.selection.map(changeSet);
    const selection = codeOffsets.size ? EditorSelection.create(mapped.ranges.map((range, index) => {
      const original = state.selection.ranges[index]!;
      const offset = original.empty ? codeOffsets.get(original.from) : undefined;
      return offset === undefined ? range : EditorSelection.cursor(changeSet.mapPos(original.from, -1) + offset);
    }), mapped.mainIndex) : mapped;
    spec = { changes: changeSet, selection };
  } else {
    const mark = ["strong", "bold"].includes(type) ? "**" : ["em", "italic"].includes(type) ? "*" : type === "strike" ? "~~" : ["code", "inline-code"].includes(type) ? "`" : "";
    if (!mark && type !== "link" && type !== "image") return null;
    spec = state.changeByRange(range => {
      let from = range.from, to = range.to;
      const selected = state.sliceDoc(from, to);
      let insert: string, start = 0, end = 0;
      if (type === "link" || type === "image") {
        const options = typeof action === "object" && (action.type === "link" || action.type === "image") ? action : undefined;
        // 用尖括号包裹目标，并编码分隔符，避免输入 URL 打断 Markdown 结构。
        const url = (options?.url ?? "https://").replace(/[\s<>\\]/g, char => encodeURIComponent(char));
        const title = options?.title ? ` "${options.title.replace(/[\\"\n\r]/g, char => char === "\n" || char === "\r" ? " " : "\\" + char)}"` : "";
        const label = selected || (type === "image" ? "图片描述" : "链接文字");
        const escaped = label.replace(/[\\\[\]]/g, "\\$&");
        const prefix = type === "image" ? "![" : "[";
        insert = `${prefix}${escaped}](<${url}>${title})`;
        start = prefix.length;
        end = start + escaped.length;
      } else {
        const inline = (text: string) => {
          if (mark === "`") {
            const wrapped = text.match(/^(`+)([\s\S]*?)\1$/);
            if (wrapped && !wrapped[2]!.includes(wrapped[1]!)) {
              let body = wrapped[2]!;
              if (body.startsWith(" ") && body.endsWith(" ") && body.trim()) body = body.slice(1, -1);
              return { text: body, offset: 0, length: body.length };
            }
            const ticks = "`".repeat(longestTicks(text) + 1);
            const pad = /^`|`$/.test(text) || (/^ .* $/.test(text) && !!text.trim()) ? " " : "";
            return { text: ticks + pad + text + pad + ticks, offset: ticks.length + pad.length, length: text.length };
          }
          if (text.startsWith(mark) && text.endsWith(mark) && text.length >= mark.length * 2) {
            const body = text.slice(mark.length, -mark.length);
            return { text: body, offset: 0, length: body.length };
          }
          return { text: mark + text + mark, offset: mark.length, length: text.length };
        };
        let surroundingMark = mark;
        let padding = 0;
        if (mark === "`") {
          const left = state.sliceDoc(0, from).match(/(`+)( ?)$/);
          const right = state.sliceDoc(to).match(/^( ?)(`+)/);
          if (left && right && left[1] === right[2] && left[2] === right[1] && longestTicks(selected) < left[1]!.length) {
            surroundingMark = left[1]!;
            padding = left[2]!.length;
          }
        }
        const width = surroundingMark.length + padding;
        const before = state.sliceDoc(Math.max(0, from - width), from);
        const after = state.sliceDoc(to, Math.min(state.doc.length, to + width));
        if (before === surroundingMark + " ".repeat(padding) && after === " ".repeat(padding) + surroundingMark && (mark !== "`" || longestTicks(selected) < surroundingMark.length)) {
          from -= width; to += width; insert = selected; end = insert.length;
        } else if (selected.includes("\n")) {
          // 行内格式逐行应用，空行保持段落边界，避免生成跨段无效语法。
          insert = selected.split("\n").map(line => line.trim() ? inline(line).text : line).join("\n");
          end = insert.length;
        } else {
          const result = inline(selected);
          insert = result.text; start = result.offset; end = start + result.length;
        }
      }
      const anchor = from + (range.anchor > range.head ? end : start);
      const head = from + (range.anchor > range.head ? start : end);
      return { changes: { from, to, insert }, range: EditorSelection.range(anchor, head) };
    });
  }
  // 一次操作（包括多行、多光标）只占一个撤销步骤，且不与前后的输入合并。
  return { ...spec, annotations: isolateHistory.of("full"), userEvent: "input.format", scrollIntoView: true };
}

/** 返回是否执行；只读状态或未知动作返回 false。 */
export function applyFormat(view: EditorView, action: FormatAction): boolean {
  const transaction = createFormatTransaction(view.state, action);
  if (!transaction) return false;
  view.dispatch(transaction);
  view.focus();
  return true;
}
