import { StateEffect, StateField, type EditorState, type Extension, type Text } from "@codemirror/state";
import { Decoration, EditorView, WidgetType, type DecorationSet } from "@codemirror/view";
import { parser, GFM } from "@lezer/markdown";
import { openTableEditor, parseTable, type MarkdownTable } from "../dialogs/table-editor";
import "./live-table.css";

export interface LiveTableOptions {
  /** 默认 true：编辑器聚焦时，选区涉及的表格显示源文。 */
  showActiveSyntax?: boolean;
}
const tableParser = parser.configure(GFM);
const tableFocus = StateEffect.define<boolean>();
/** 超大文档保留源码，不进行同步全文表格解析。 */
export const MAX_TABLE_PREVIEW_CHARACTERS = 1_000_000;
interface TableBlock { from: number; to: number; raw: string; table: MarkdownTable }
const tableCache = new WeakMap<Text, TableBlock[]>();
function collectTables(state: EditorState): TableBlock[] {
  const cached = tableCache.get(state.doc);
  if (cached) return cached;
  const blocks: TableBlock[] = [];
  tableCache.set(state.doc, blocks);
  if (state.doc.length > MAX_TABLE_PREVIEW_CHARACTERS) return blocks;
  tableParser.parse(state.doc.toString()).iterate({ enter(node) {
    if (node.name !== "Table") return;
    const first = state.doc.lineAt(node.from), last = state.doc.lineAt(node.to);
    // 只替换顶层完整行，容器前缀始终交由原来的 Markdown 编辑器处理。
    if (node.node.parent?.name !== "Document" || node.from !== first.from || node.to !== last.to) return false;
    const raw = state.doc.sliceString(node.from, node.to);
    const table = parseTable(raw);
    if (table) blocks.push({ from: node.from, to: node.to, raw, table });
    return false;
  } });
  return blocks;
}

class TablePreview extends WidgetType {
  constructor(readonly source: string, readonly position: number, readonly table: MarkdownTable) { super(); }
  eq(other: TablePreview) { return this.source === other.source && this.position === other.position; }
  toDOM(view: EditorView): HTMLElement {
    const doc = view.dom.ownerDocument;
    const wrapper = doc.createElement("div");
    wrapper.className = "lm-table-preview";
    const button = doc.createElement("button");
    button.type = "button";
    button.className = "lm-table-edit";
    button.textContent = "编辑表格";
    const edit = () => {
      // 先定位源表格，再同步打开编辑器；不更改文档，保留撤销历史。
      view.dispatch({ selection: { anchor: this.position } });
      openTableEditor(view);
    };
    button.addEventListener("click", edit);
    const table = doc.createElement("table");
    table.className = "table-grid";
    table.setAttribute("aria-label", "Markdown 表格");
    const head = doc.createElement("thead");
    const body = doc.createElement("tbody");
    const appendRow = (values: string[], parent: HTMLElement, heading: boolean) => {
      const row = doc.createElement("tr");
      values.forEach((value, column) => {
        const cell = doc.createElement(heading ? "th" : "td");
        if (heading) cell.setAttribute("scope", "col");
        // 使用 textContent：任意 HTML、链接和图片源文都不会执行或发起请求。
        cell.textContent = value.replace(/\\\|/g, "|");
        const alignment = this.table.alignments[column];
        if (alignment) cell.style.textAlign = alignment;
        row.append(cell);
      });
      parent.append(row);
    };
    appendRow(this.table.headers, head, true);
    this.table.rows.forEach(row => appendRow(row, body, false));
    table.append(head, body);
    table.addEventListener("dblclick", edit);
    wrapper.append(button, table);
    return wrapper;
  }
  ignoreEvent() { return true; }
}

/** 整个表格的 block 替换只能由 StateField 提供，不能由 ViewPlugin 提供。 */
export function buildLiveTableDecorations(state: EditorState, focused: boolean, options: LiveTableOptions = {}): DecorationSet {
  const ranges: ReturnType<Decoration["range"]>[] = [];
  for (const block of collectTables(state)) {
    const selected = focused && options.showActiveSyntax !== false && state.selection.ranges.some(selection => {
      return selection.from <= block.to && selection.to >= block.from;
    });
    if (selected) continue;
    ranges.push(Decoration.replace({ block: true, widget: new TablePreview(block.raw, block.from, block.table) }).range(block.from, block.to));
  }
  return Decoration.set(ranges, true);
}

export function liveTable(options: LiveTableOptions = {}): Extension {
  const field = StateField.define<{ focused: boolean; decorations: DecorationSet }>({
    create(state) { return { focused: false, decorations: buildLiveTableDecorations(state, false, options) }; },
    update(value, transaction) {
      let focused = value.focused;
      for (const effect of transaction.effects) if (effect.is(tableFocus)) focused = effect.value;
      if (!transaction.docChanged && !transaction.selection && focused === value.focused) return value;
      return { focused, decorations: buildLiveTableDecorations(transaction.state, focused, options) };
    },
    provide: field => EditorView.decorations.from(field, value => value.decorations),
  });
  return [field, EditorView.updateListener.of(update => {
    if (update.focusChanged && update.state.field(field).focused !== update.view.hasFocus) {
      // 更新监听器中派发焦点 effect，让 StateField 能正确控制 active table。
      update.view.dispatch({ effects: tableFocus.of(update.view.hasFocus) });
    }
  })];
}
