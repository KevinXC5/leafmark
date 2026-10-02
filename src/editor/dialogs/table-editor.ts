import type { EditorView } from "@codemirror/view";
import { isolateHistory } from "@codemirror/commands";
import { parser, GFM } from "@lezer/markdown";
import "./interaction-details.css";

export type TableAlignment = "left" | "center" | "right" | null;
export interface MarkdownTable {
  /** 单元格使用 Markdown 源文，已有的转义和 inline code 均保留。 */
  headers: string[];
  rows: string[][];
  alignments: TableAlignment[];
}
const tableParser = parser.configure(GFM);

/** GFM 的管道即使位于 inline code 中，也必须转义才能属于单元格。 */
export function splitTableRow(line: string): string[] {
  const cells: string[] = [];
  let start = 0;
  let escaped = false;
  const delimiters: number[] = [];
  for (let i = 0; i < line.length; i++) {
    const character = line[i];
    if (character === "|" && !escaped) delimiters.push(i);
    escaped = character === "\\" && !escaped;
  }
  for (const position of delimiters) {
    cells.push(line.slice(start, position).trim());
    start = position + 1;
  }
  cells.push(line.slice(start).trim());
  if (delimiters.length && line.slice(0, delimiters[0]).trim() === "") cells.shift();
  if (delimiters.length && line.slice(delimiters.at(-1)! + 1).trim() === "") cells.pop();
  return cells;
}

/** 解析单个 GFM 表格；无效语法、代码块或混入其他块时返回 null。 */
export function parseTable(markdown: string): MarkdownTable | null {
  const source = markdown.replace(/\r\n?/g, "\n").trimEnd();
  const root = tableParser.parse(source).topNode;
  const node = root.firstChild;
  if (!node || node.name !== "Table" || node.nextSibling || node.to !== source.length) return null;
  const lines = source.split("\n");
  if (lines.length < 2) return null;
  const headers = splitTableRow(lines[0]!);
  const separators = splitTableRow(lines[1]!);
  if (!headers.length || headers.length !== separators.length || separators.some(cell => !/^:?-+:?$/.test(cell))) return null;
  const alignments: TableAlignment[] = separators.map(cell => cell.startsWith(":") ? (cell.endsWith(":") ? "center" : "left") : cell.endsWith(":") ? "right" : null);
  const rows = lines.slice(2).map(line => {
    const cells = splitTableRow(line);
    return headers.map((_, column) => cells[column] ?? "");
  });
  return { headers, rows, alignments };
}

function serializeCell(value: string): string {
  let result = "";
  let escaped = false;
  for (const character of value.replace(/\r\n?|\n/g, "<br>")) {
    result += character === "|" && !escaped ? "\\|" : character;
    escaped = character === "\\" && !escaped;
  }
  // 结尾的奇数个反斜杠不能转义用于分列的管道。
  if (escaped) result += "\\";
  return result.trim();
}

export function serializeTable(table: MarkdownTable): string {
  if (!table.headers.length) throw new Error("表格至少需要一列");
  const width = table.headers.length;
  const row = (cells: string[]) => `| ${Array.from({ length: width }, (_, i) => serializeCell(cells[i] ?? "")).join(" | ")} |`;
  const markers = table.headers.map((_, i) => {
    switch (table.alignments[i]) {
      case "left": return ":---";
      case "center": return ":---:";
      case "right": return "---:";
      default: return "---";
    }
  });
  return [row(table.headers), row(markers), ...table.rows.map(row)].join("\n");
}

/** 光标不在 GFM 表格内时返回 false；成功打开对话框时返回 true。 */
export function openTableEditor(view: EditorView): boolean {
  const snapshot = view.state.doc;
  const position = view.state.selection.main.head;
  const tree = tableParser.parse(snapshot.toString());
  let node = tree.resolveInner(position, -1);
  while (node.name !== "Table" && node.parent) node = node.parent;
  if (node.name !== "Table") {
    // 位于表格起点时向右解析，否则会落在前一个块或 Document 节点。
    node = tree.resolveInner(position, 1);
    while (node.name !== "Table" && node.parent) node = node.parent;
  }
  if (node.name !== "Table") return false;
  const from = node.from;
  const to = node.to;
  const firstLine = snapshot.lineAt(from);
  const continuation = snapshot.lineAt(Math.min(to, firstLine.to + 1)).text.match(/^[\t ]*(?:>[\t ]*)*/)?.[0] ?? "";
  // 引用和列表中的表格保留容器前缀，第一行前缀位于替换区间外。
  const raw = snapshot.sliceString(from, to).split("\n").map((line, index) => index ? line.slice(continuation.length) : line).join("\n");
  const table = parseTable(raw);
  if (!table) return false;
  const doc = view.dom.ownerDocument;
  const dialog = doc.createElement("dialog");
  dialog.className = "lm-dialog lm-table-dialog";
  const form = doc.createElement("form");
  const title = doc.createElement("h2");
  title.textContent = "编辑表格";
  dialog.setAttribute("aria-label", title.textContent);
  const grid = doc.createElement("table");
  grid.className = "table-grid";
  const toolbar = doc.createElement("div");
  toolbar.className = "lm-table-tools";
  const actions = doc.createElement("div");
  actions.className = "lm-detail-actions";
  const header = doc.createElement("header");
  header.className = "lm-detail-header";
  const dimensions = doc.createElement("span");
  header.append(title, dimensions);
  const body = doc.createElement("div");
  body.className = "lm-detail-body";
  const status = doc.createElement("div");
  status.className = "lm-table-status";
  const selected = doc.createElement("span");
  const draft = doc.createElement("span");
  draft.textContent = "待应用";
  status.append(selected, draft);
  const gridScroll = doc.createElement("div");
  gridScroll.className = "lm-table-grid-scroll";
  gridScroll.append(grid);
  const cellField = doc.createElement("label");
  cellField.className = "lm-detail-field";
  const cellCaption = doc.createElement("span");
  cellCaption.textContent = "单元格内容";
  const cellRow = doc.createElement("div");
  cellRow.className = "lm-detail-input-row";
  const cellInput = doc.createElement("input");
  cellInput.type = "text";
  cellInput.setAttribute("aria-label", "单元格内容");
  cellRow.append(cellInput);
  cellField.append(cellCaption, cellRow);
  const error = doc.createElement("p");
  error.setAttribute("role", "alert");
  let activeRow = 0;
  let activeColumn = 0;
  let finished = false;
  const close = () => {
    if (finished) return;
    finished = true;
    dialog.close();
    dialog.remove();
    view.focus();
  };
  const button = (label: string, parent: HTMLElement, action: () => void) => {
    const element = doc.createElement("button");
    element.type = "button";
    element.textContent = label;
    element.addEventListener("click", action);
    parent.append(element);
    return element;
  };
  const allRows = () => [table.headers, ...table.rows];
  const states: Array<{ button: HTMLButtonElement; disabled: () => boolean }> = [];
  const alignmentButtons: Array<{ button: HTMLButtonElement; alignment: TableAlignment }> = [];
  const syncSelection = () => {
    selected.textContent = `已选：${activeRow ? `第 ${activeRow} 行` : "表头"} · 第 ${activeColumn + 1} 列`;
    dimensions.textContent = `${table.headers.length} 列 · ${table.rows.length} 行`;
    cellInput.value = allRows()[activeRow]![activeColumn]!;
    grid.querySelectorAll<HTMLInputElement>("input[data-row]").forEach(input => {
      const active = Number(input.dataset.row) === activeRow && Number(input.dataset.column) === activeColumn;
      input.parentElement!.classList.toggle("selected", active);
      input.parentElement!.parentElement!.classList.toggle("selected-row", Number(input.dataset.row) === activeRow);
      input.parentElement!.setAttribute("aria-selected", String(active));
    });
    rowTools.previousElementSibling!.textContent = activeRow ? `第 ${activeRow} 行` : "表头行";
    columnTools.previousElementSibling!.textContent = `第 ${activeColumn + 1} 列`;
    states.forEach(({ button, disabled }) => { button.disabled = disabled(); });
    alignmentButtons.forEach(({ button, alignment }) => button.setAttribute("aria-pressed", String((table.alignments[activeColumn] ?? null) === alignment)));
  };
  const commitCell = () => {
    allRows()[activeRow]![activeColumn] = cellInput.value;
    const input = grid.querySelector<HTMLInputElement>(`input[data-row="${activeRow}"][data-column="${activeColumn}"]`);
    if (input) input.value = cellInput.value;
  };
  cellInput.addEventListener("input", commitCell);
  cellInput.addEventListener("keydown", event => { if (event.key === "Enter") { event.preventDefault(); commitCell(); } });
  button("应用", cellRow, commitCell);
  const group = (label: string, parent: HTMLElement = toolbar) => {
    const section = doc.createElement("section");
    section.className = "lm-table-tool-group";
    const caption = doc.createElement("span");
    caption.textContent = label;
    const row = doc.createElement("div");
    row.className = "lm-table-tool-buttons";
    section.append(caption, row);
    parent.append(section);
    return row;
  };
  const rowTools = group("行操作");
  const columnTools = group("列操作");
  const moving = doc.createElement("div");
  moving.className = "lm-table-moving";
  toolbar.append(moving);
  const rowMove = group("移动行", moving);
  const columnMove = group("移动列", moving);
  const alignTools = group("列对齐", moving);
  const controlled = (label: string, parent: HTMLElement, action: () => void, disabled: () => boolean) => {
    const control = button(label, parent, action);
    states.push({ button: control, disabled });
    return control;
  };
  const render = () => {
    grid.replaceChildren();
    allRows().forEach((row, rowIndex) => {
      const tr = doc.createElement("tr");
      row.forEach((value, column) => {
        const td = doc.createElement(rowIndex === 0 ? "th" : "td");
        const input = doc.createElement("input");
        input.type = "text";
        input.className = "form-field";
        input.value = value;
        input.dataset.row = String(rowIndex);
        input.dataset.column = String(column);
        input.setAttribute("aria-label", `${rowIndex === 0 ? "表头" : `第 ${rowIndex} 行`}，第 ${column + 1} 列`);
        input.style.textAlign = table.alignments[column] ?? "left";
        input.addEventListener("focus", () => { activeRow = rowIndex; activeColumn = column; syncSelection(); });
        input.addEventListener("input", () => { row[column] = input.value; cellInput.value = input.value; });
        td.append(input);
        tr.append(td);
      });
      grid.append(tr);
    });
    syncSelection();
  };
  const refresh = () => {
    activeRow = Math.min(activeRow, table.rows.length);
    activeColumn = Math.min(activeColumn, table.headers.length - 1);
    render();
    grid.querySelector<HTMLInputElement>(`input[data-row="${activeRow}"][data-column="${activeColumn}"]`)?.focus();
  };
  const insertRow = (above: boolean) => {
    const index = above ? Math.max(0, activeRow - 1) : activeRow;
    table.rows.splice(index, 0, table.headers.map(() => ""));
    activeRow = index + 1;
    refresh();
  };
  controlled("上方插入", rowTools, () => insertRow(true), () => activeRow === 0);
  button("下方插入", rowTools, () => insertRow(false));
  controlled("删除行", rowTools, () => { if (activeRow > 0) { table.rows.splice(activeRow - 1, 1); refresh(); } }, () => activeRow === 0);
  const insertColumn = (left: boolean) => {
    const index = activeColumn + (left ? 0 : 1);
    allRows().forEach(row => row.splice(index, 0, ""));
    table.alignments.splice(index, 0, null);
    activeColumn = index;
    refresh();
  };
  button("左侧插入", columnTools, () => insertColumn(true));
  button("右侧插入", columnTools, () => insertColumn(false));
  controlled("删除列", columnTools, () => {
    if (table.headers.length <= 1) return;
    allRows().forEach(row => row.splice(activeColumn, 1));
    table.alignments.splice(activeColumn, 1);
    refresh();
  }, () => table.headers.length <= 1);
  const moveRow = (direction: number) => {
    const target = activeRow + direction;
    // 表头固定，正文行只在正文区域内移动。
    if (activeRow < 1 || target < 1 || target > table.rows.length) return;
    const rows = table.rows;
    [rows[activeRow - 1], rows[target - 1]] = [rows[target - 1]!, rows[activeRow - 1]!];
    activeRow = target;
    refresh();
  };
  const moveColumn = (direction: number) => {
    const target = activeColumn + direction;
    if (target < 0 || target >= table.headers.length) return;
    allRows().forEach(row => { [row[activeColumn], row[target]] = [row[target]!, row[activeColumn]!]; });
    [table.alignments[activeColumn], table.alignments[target]] = [table.alignments[target]!, table.alignments[activeColumn]!];
    activeColumn = target;
    refresh();
  };
  controlled("↑", rowMove, () => moveRow(-1), () => activeRow <= 1).setAttribute("aria-label", "行上移");
  controlled("↓", rowMove, () => moveRow(1), () => activeRow < 1 || activeRow >= table.rows.length).setAttribute("aria-label", "行下移");
  controlled("←", columnMove, () => moveColumn(-1), () => activeColumn <= 0).setAttribute("aria-label", "列左移");
  controlled("→", columnMove, () => moveColumn(1), () => activeColumn >= table.headers.length - 1).setAttribute("aria-label", "列右移");
  for (const [label, alignment] of [["左对齐", "left"], ["居中", "center"], ["右对齐", "right"]] as const) {
    const control = button(label, alignTools, () => {
      // 再次点击当前对齐方式时恢复为默认对齐。
      table.alignments[activeColumn] = table.alignments[activeColumn] === alignment ? null : alignment;
      refresh();
    });
    control.setAttribute("aria-label", label);
    alignmentButtons.push({ button: control, alignment });
  }
  const hint = doc.createElement("p");
  hint.className = "lm-detail-hint";
  hint.textContent = "Tab 下一单元格 · Shift Tab 上一单元格";
  button("取消", actions, close);
  const save = doc.createElement("button");
  save.type = "submit";
  save.className = "primary";
  save.textContent = "完成";
  actions.append(save);
  error.className = "lm-detail-error";
  body.append(status, gridScroll, cellField, toolbar, error, hint, actions);
  form.append(header, body);
  dialog.append(form);
  form.addEventListener("submit", event => {
    event.preventDefault();
    if (view.state.doc !== snapshot) { error.textContent = "文档已变化，请关闭后重新打开表格编辑器。"; return; }
    const insert = serializeTable(table).replace(/\n/g, `\n${continuation}`);
    if (insert !== snapshot.sliceString(from, to)) {
      view.dispatch({ changes: { from, to, insert }, selection: { anchor: from }, annotations: isolateHistory.of("full"), userEvent: "input.table" });
    }
    close();
  });
  dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
  dialog.addEventListener("close", close);
  render();
  doc.body.append(dialog);
  dialog.showModal();
  grid.querySelector("input")?.focus();
  return true;
}
