import { StateEffect, StateField, type EditorState, type Extension, type Text } from "@codemirror/state";
import { Decoration, EditorView, WidgetType, type DecorationSet } from "@codemirror/view";
import { parser, GFM } from "@lezer/markdown";
import { createElement, Ellipsis } from "lucide";
import { isolateHistory } from "@codemirror/commands";
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
    button.className = "lm-table-actions";
    button.setAttribute("aria-label", "表格操作");
    button.setAttribute("aria-haspopup", "menu");
    button.setAttribute("aria-expanded", "false");
    button.title = "表格操作";
    button.append(createElement(Ellipsis, { width: 16, height: 16, "aria-hidden": "true" }));
    let activeRow = 0;
    let menu: HTMLElement | null = null;
    const closeMenu = (restoreFocus = false) => {
      menu?.remove(); menu = null;
      wrapper.classList.remove("lm-table-menu-open");
      button.setAttribute("aria-expanded", "false");
      if (restoreFocus) button.focus({ preventScroll: true });
    };
    // 保留原表格的源码格式，只对目标行插入；删除与插行均可独立撤销。
    const change = (action: "above" | "below" | "delete") => {
      if (view.state.doc.sliceString(this.position, this.position + this.source.length) !== this.source) return;
      const lines = this.source.split("\n");
      if (action !== "delete") {
        const index = Math.min(lines.length, activeRow + 2 + (action === "below" && this.table.rows.length ? 1 : 0));
        lines.splice(index, 0, `| ${this.table.headers.map(() => "").join(" | ")} |`);
      }
      const insert = action === "delete" ? "" : lines.join("\n");
      closeMenu();
      view.dispatch({ changes: { from: this.position, to: this.position + this.source.length, insert }, annotations: isolateHistory.of("full"), userEvent: "input" });
      view.focus();
    };
    const edit = () => {
      // 先定位源表格，再同步打开编辑器；不更改文档，保留撤销历史。
      view.dispatch({ selection: { anchor: this.position } });
      openTableEditor(view);
    };
    button.addEventListener("click", () => {
      if (menu) { closeMenu(); return; }
      menu = doc.createElement("div");
      menu.className = "lm-table-menu";
      menu.setAttribute("role", "menu");
      menu.setAttribute("aria-label", "表格操作");
      wrapper.classList.add("lm-table-menu-open");
      button.setAttribute("aria-expanded", "true");
      const actions: [string, () => void][] = [
        ["在上方插入行", () => change("above")],
        ["在下方插入行", () => change("below")],
        ["编辑表格", () => { closeMenu(); edit(); }],
        ["删除表格", () => change("delete")],
      ];
      for (const [label, action] of actions) {
        const item = doc.createElement("button");
        item.type = "button"; item.textContent = label; item.setAttribute("role", "menuitem");
        if (label === "删除表格") item.className = "lm-table-delete";
        item.addEventListener("click", action); menu.append(item);
      }
      menu.addEventListener("keydown", event => {
        const items = [...menu!.querySelectorAll<HTMLButtonElement>("button")];
        const index = items.indexOf(doc.activeElement as HTMLButtonElement);
        if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); closeMenu(true); }
        else if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
          event.preventDefault();
          const next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
          items[next]?.focus();
        }
      });
      wrapper.append(menu);
      menu.querySelector<HTMLButtonElement>("button")?.focus({ preventScroll: true });
    });
    wrapper.addEventListener("focusout", event => {
      if (!(event.relatedTarget instanceof doc.defaultView!.Node) || !wrapper.contains(event.relatedTarget)) closeMenu();
    });
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
    this.table.rows.forEach((row, index) => {
      appendRow(row, body, false);
      body.lastElementChild!.addEventListener("mouseenter", () => { if (!menu) activeRow = index; });
    });
    table.append(head, body);
    table.addEventListener("dblclick", edit);
    const scroll = doc.createElement("div");
    scroll.className = "lm-table-scroll";
    scroll.append(table);
    wrapper.append(button, scroll);
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
