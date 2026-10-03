import { StateEffect, StateField, type EditorState, type Extension, type Text } from "@codemirror/state";
import { Decoration, EditorView, WidgetType, type DecorationSet } from "@codemirror/view";
import { parser, GFM } from "@lezer/markdown";
import hljs from "highlight.js/lib/common";
import katex from "katex";
import { Copy, createElement, Pencil } from "lucide";
import { liveBlockRanges } from "./live-block-ranges";
import { parseWikiReference } from "../../markdown/obsidian-syntax";
import "./live-blocks.css";

export interface LiveBlocksOptions {
  /** 默认 true：聚焦且光标位于块内时显示源码；非空选区始终保留源码。 */
  showActiveSyntax?: boolean;
  /** 可注入原生剪贴板桥接；未提供时使用浏览器 Clipboard API。 */
  copyText?: (text: string) => Promise<void>;
}
interface BlockRange { from: number; to: number }
interface Block extends BlockRange { source: string; content: string; language: string; math: boolean; inline?: boolean }

const blockFocus = StateEffect.define<boolean>();
const editBlock = StateEffect.define<number>();

/** 超过此 UTF-16 字符数时保留源码，避免全文同步解析阻塞编辑。 */
export const MAX_BLOCK_PREVIEW_CHARACTERS = 1_000_000;
const blockCache = new WeakMap<Text, Block[]>();

function collectBlocks(state: EditorState): Block[] {
  const cached = blockCache.get(state.doc);
  if (cached) return cached;
  const blocks: Block[] = [];
  blockCache.set(state.doc, blocks);
  if (state.doc.length > MAX_BLOCK_PREVIEW_CHARACTERS) return blocks;
  // 用独立语法树识别顶层块，不依赖宿主是否安装 Markdown language。
  const blockParser = parser.configure([GFM, {
    defineNodes: [{ name: "LiveMathBlock", block: true }],
    parseBlock: [{
      name: "LiveMathBlock", before: "FencedCode",
      parse(cx, line) {
        if (cx.parentType().name !== "Document" || !/^\$\$[ \t]*$/.test(line.text)) return false;
        const first = state.doc.lineAt(cx.lineStart);
        let close = first.number + 1;
        while (close <= state.doc.lines && !/^\$\$[ \t]*$/.test(state.doc.line(close).text)) close++;
        // 未闭合公式不吞掉后续文档，继续作为普通 Markdown 编辑。
        if (close > state.doc.lines) return false;
        const end = state.doc.line(close).to;
        while (cx.lineStart <= end && cx.nextLine()) { /* 消费完整公式围栏。 */ }
        cx.addElement(cx.elt("LiveMathBlock", first.from, end));
        return true;
      },
      endLeaf: (cx, line) => cx.parentType().name === "Document" && /^\$\$[ \t]*$/.test(line.text),
    }],
  }]);
  const tree = blockParser.parse(state.doc.toString());
  tree.iterate({ enter(node) {
    if (node.name !== "FencedCode" && node.name !== "LiveMathBlock") return;
    const first = state.doc.lineAt(node.from);
    const last = state.doc.lineAt(node.to);
    // 引用、列表、缩进代码不进行跨行替换，防止吞掉容器前缀。
    if (node.node.parent?.name !== "Document" || node.from !== first.from || node.to !== last.to) return false;
    const math = node.name === "LiveMathBlock";
    const marks = node.node.getChildren("CodeMark");
    // 编辑中未闭合的代码块保留源文，不把最后一行误当作关闭围栏。
    if (!math && marks.length < 2) return false;
    const info = node.node.getChild("CodeInfo");
    const language = info ? state.doc.sliceString(info.from, info.to).trim().split(/\s+/)[0]!.toLowerCase() : "";
    const content = first.number === last.number ? "" : state.doc.sliceString(first.to + 1, last.from);
    blocks.push({ from: node.from, to: node.to, source: state.doc.sliceString(node.from, node.to), content, language, math });
    return false;
  } });
  // 仅扫描正常正文，跳过代码、链接、HTML 等语法；转义美元和货币数字不识别为公式。
  const excluded: BlockRange[] = [...blocks];
  tree.iterate({ enter(node) {
    if (["FencedCode", "CodeBlock", "InlineCode", "HTMLBlock", "HTMLTag", "Link", "Image", "Autolink"].includes(node.name)) {
      excluded.push({ from: node.from, to: node.to });
      return false;
    }
  } });
  for (let number = 1; number <= state.doc.lines; number++) {
    const line = state.doc.line(number);
    const escaped = (index: number) => {
      let slashes = 0;
      while (index > 0 && line.text[--index] === "\\") slashes++;
      return slashes % 2 !== 0;
    };
    for (let start = 0; start < line.text.length; start++) {
      const wiki = !escaped(start) ? parseWikiReference(line.text, start) : undefined;
      if (wiki) { start += wiki.length - 1; continue; }
      if (line.text[start] !== "$" || escaped(start) || line.text[start - 1] === "$" || line.text[start + 1] === "$" || /\s/.test(line.text[start + 1] ?? " ")) continue;
      let close = start + 1;
      while (close < line.text.length && (line.text[close] !== "$" || escaped(close))) close++;
      if (close === line.text.length || /\s/.test(line.text[close - 1]!) || /\d|\$/.test(line.text[close + 1] ?? "")) continue;
      const from = line.from + start, to = line.from + close + 1;
      if (excluded.some(range => from < range.to && to > range.from)) continue;
      const content = line.text.slice(start + 1, close);
      // 无效 TeX 直接留在正文中，避免插入错误 widget 干扰行布局。
      try { renderMath(content, false); } catch { continue; }
      blocks.push({ from, to, source: line.text.slice(start, close + 1), content, language: "", math: true, inline: true });
      start = close;
    }
  }
  return blocks;
}

function renderMath(source: string, displayMode: boolean): string {
  return katex.renderToString(source, { output: "mathml", displayMode, trust: false, throwOnError: true, strict: "error", maxExpand: 1000, maxSize: 20 });
}

class InlineMathPreview extends WidgetType {
  constructor(readonly block: Block) { super(); }
  eq(other: InlineMathPreview) { return this.block.source === other.block.source && this.block.from === other.block.from; }
  toDOM(view: EditorView): HTMLElement {
    const span = view.dom.ownerDocument.createElement("span");
    span.className = "lm-inline-math";
    span.innerHTML = renderMath(this.block.content, false);
    span.addEventListener("mousedown", event => {
      event.preventDefault();
      view.dispatch({ effects: editBlock.of(this.block.from), selection: { anchor: this.block.from + 1 } });
      view.focus();
    });
    return span;
  }
  ignoreEvent() { return true; }
}

const removedWidgets = new WeakSet<HTMLElement>();

class BlockPreview extends WidgetType {
  constructor(readonly block: Block, readonly copyText?: LiveBlocksOptions["copyText"]) { super(); }
  eq(other: BlockPreview) { return this.block.source === other.block.source && this.block.from === other.block.from && this.copyText === other.copyText; }
  toDOM(view: EditorView): HTMLElement {
    const doc = view.dom.ownerDocument;
    const wrapper = doc.createElement("div");
    wrapper.className = `lm-block-preview ${this.block.math ? "lm-math-preview" : "lm-code-preview"}`;
    const toolbar = doc.createElement("div");
    toolbar.className = "lm-block-toolbar";
    const icon = (button: HTMLButtonElement, node: typeof Copy, label: string) => {
      button.type = "button"; button.title = label; button.setAttribute("aria-label", label);
      button.append(createElement(node, { width: 13, height: 13, "aria-hidden": "true" }));
    };
    const edit = doc.createElement("button");
    icon(edit, Pencil, this.block.math ? "编辑公式源码" : "编辑代码源码");
    const reveal = () => {
      if (removedWidgets.has(wrapper)) return;
      const anchor = view.state.doc.lineAt(this.block.from).to + 1;
      view.dispatch({ effects: editBlock.of(this.block.from), selection: { anchor }, scrollIntoView: true });
      view.focus();
    };
    edit.addEventListener("click", reveal);
    const copy = doc.createElement("button");
    icon(copy, Copy, this.block.math ? "复制公式源码" : "复制代码");
    const status = doc.createElement("span");
    status.className = "lm-block-status";
    status.setAttribute("role", "status");
    copy.addEventListener("click", async () => {
      copy.disabled = true;
      try {
        if (this.copyText) await this.copyText(this.block.content);
        else {
          const clipboard = doc.defaultView?.navigator.clipboard;
          if (!clipboard?.writeText) throw new Error("Clipboard API 不可用");
          await clipboard.writeText(this.block.content);
        }
        if (wrapper.isConnected && !removedWidgets.has(wrapper)) status.textContent = "已复制";
      } catch {
        if (wrapper.isConnected && !removedWidgets.has(wrapper)) status.textContent = "复制失败，请点击编辑后手动复制。";
      } finally {
        if (wrapper.isConnected && !removedWidgets.has(wrapper)) copy.disabled = false;
      }
    });
    // 控件点击不改变编辑器选区，复制时不会意外切回源码。
    toolbar.addEventListener("mousedown", event => event.preventDefault());
    // 语言名来自围栏信息串，只随悬浮工具栏出现；公式和纯文本不加标签。
    if (!this.block.math && this.block.language) {
      const label = doc.createElement("span");
      label.className = "lm-block-language";
      label.textContent = this.block.language;
      toolbar.append(label);
    }
    toolbar.append(status, copy, edit);
    const content = doc.createElement("div");
    content.className = "lm-block-content";
    content.addEventListener("dblclick", reveal);
    if (this.block.math) {
      try {
        // 原生 MathML 无需 KaTeX 字体/CSS；关闭信任并限制宏展开和尺寸。
        content.innerHTML = renderMath(this.block.content, true);
      } catch {
        status.textContent = "公式渲染失败，请编辑检查语法。";
        this.appendCode(content, doc);
      }
    } else this.appendCode(content, doc);
    wrapper.append(toolbar, content);
    return wrapper;
  }
  private appendCode(parent: HTMLElement, doc: Document) {
    const pre = doc.createElement("pre");
    const code = doc.createElement("code");
    code.className = "hljs";
    code.textContent = this.block.content;
    // 未知语言安全退回纯文本，仅插入 highlight.js 自身生成的转义 HTML。
    if (!this.block.math && this.block.language && hljs.getLanguage(this.block.language)) {
      try { code.innerHTML = hljs.highlight(this.block.content, { language: this.block.language, ignoreIllegals: true }).value; }
      catch { /* 保留安全的 textContent 回退。 */ }
    }
    pre.append(code);
    parent.append(pre);
  }
  destroy(dom: HTMLElement) {
    // Clipboard Promise 无法取消，销毁后仅忽略其回调，避免修改失效 widget。
    removedWidgets.add(dom);
  }
  ignoreEvent() { return true; }
}

function decorate(state: EditorState, blocks: readonly Block[], focused: boolean, options: LiveBlocksOptions, editing: number | null): DecorationSet {
  const ranges: ReturnType<Decoration["range"]>[] = [];
  for (const block of blocks) {
    const selected = state.selection.ranges.some(selection => {
      if (!selection.empty) return selection.from <= block.to && selection.to >= block.from;
      if (!focused || options.showActiveSyntax === false) return false;
      const line = state.doc.lineAt(selection.head);
      return block.inline ? block.from >= line.from && block.to <= line.to : selection.head >= block.from && selection.head <= block.to;
    });
    if (selected || editing === block.from) continue;
    ranges.push(Decoration.replace({ block: !block.inline, widget: block.inline ? new InlineMathPreview(block) : new BlockPreview(block, options.copyText) }).range(block.from, block.to));
  }
  return Decoration.set(ranges, true);
}

export function liveBlocks(options: LiveBlocksOptions = {}): Extension {
  const field = StateField.define<{ blocks: Block[]; focused: boolean; editing: number | null; decorations: DecorationSet }>({
    create(state) {
      const blocks = collectBlocks(state);
      return { blocks, focused: false, editing: null, decorations: decorate(state, blocks, false, options, null) };
    },
    update(value, tr) {
      let focused = value.focused;
      let editing = value.editing === null ? null : tr.changes.mapPos(value.editing);
      for (const effect of tr.effects) {
        if (effect.is(blockFocus)) focused = effect.value;
        if (effect.is(editBlock)) editing = effect.value;
      }
      const blocks = tr.docChanged ? collectBlocks(tr.state) : value.blocks;
      const edited = blocks.find(block => block.from === editing);
      // 显式编辑只持续到选区离开该块或编辑器失焦，支持强制预览模式下编辑。
      if ((value.focused && !focused) || !edited || !tr.state.selection.ranges.some(range => range.from <= edited.to && range.to >= edited.from)) editing = null;
      if (!tr.docChanged && !tr.selection && focused === value.focused && editing === value.editing) return value;
      return { blocks, focused, editing, decorations: decorate(tr.state, blocks, focused, options, editing) };
    },
    provide: field => [
      EditorView.decorations.from(field, value => value.decorations),
      liveBlockRanges.from(field, value => {
        const ranges: BlockRange[] = [];
        value.decorations.between(0, Number.MAX_SAFE_INTEGER, (from, to) => { ranges.push({ from, to }); });
        return ranges;
      }),
    ],
  });
  return [field, EditorView.updateListener.of(update => {
    if (update.focusChanged && update.state.field(field).focused !== update.view.hasFocus) {
      update.view.dispatch({ effects: blockFocus.of(update.view.hasFocus) });
    }
  })];
}
