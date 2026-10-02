import { EditorView, Decoration, ViewPlugin, WidgetType, type DecorationSet, type ViewUpdate } from "@codemirror/view";
import { syntaxTree } from "@codemirror/language";
import { isSafeMarkdownUrl } from "./insert-dialogs";
import { liveBlockRanges } from "./live-block-ranges";
import "./live-markdown.css";

export interface LiveMarkdownOptions {
  /** 默认 true：编辑器聚焦时，选区涉及的行显示 Markdown 源码。 */
  showActiveSyntax?: boolean;
  /** 从主代理缓存同步读取图片资源；返回的 URL 仍需通过预览安全校验。 */
  resolveImage?: (url: string) => string | undefined;
  /** 缓存未命中时通知主代理异步读取，缓存就绪后重新配置扩展即可刷新。 */
  onImageNeeded?: (url: string) => void;
}

class TaskBox extends WidgetType {
  constructor(readonly checked: boolean, readonly position: number) { super(); }
  eq(other: TaskBox) { return this.checked === other.checked && this.position === other.position; }
  toDOM(view: EditorView) {
    const input = view.dom.ownerDocument.createElement("input");
    input.type = "checkbox";
    input.checked = this.checked;
    input.className = "task-box";
    input.setAttribute("aria-label", this.checked ? "标记为未完成" : "标记为完成");
    input.addEventListener("mousedown", event => event.preventDefault());
    input.addEventListener("click", () => {
      view.dispatch({ changes: { from: this.position + 1, to: this.position + 2, insert: this.checked ? " " : "x" }, userEvent: "input.task" });
      view.focus();
    });
    return input;
  }
  ignoreEvent() { return true; }
}

class Bullet extends WidgetType {
  eq(other: WidgetType) { return other instanceof Bullet; }
  toDOM(view: EditorView) {
    const span = view.dom.ownerDocument.createElement("span");
    span.className = "lm-list-bullet";
    span.textContent = "•";
    span.setAttribute("aria-hidden", "true");
    return span;
  }
}
class Rule extends WidgetType {
  eq(other: WidgetType) { return other instanceof Rule; }
  toDOM(view: EditorView) {
    const span = view.dom.ownerDocument.createElement("span");
    span.className = "lm-horizontal-rule";
    span.setAttribute("role", "separator");
    return span;
  }
}
class ImagePreview extends WidgetType {
  constructor(readonly url: string, readonly alt: string, readonly title: string) { super(); }
  eq(other: ImagePreview) { return this.url === other.url && this.alt === other.alt && this.title === other.title; }
  toDOM(view: EditorView) {
    const wrapper = view.dom.ownerDocument.createElement("span");
    wrapper.className = "lm-image-preview";
    const image = view.dom.ownerDocument.createElement("img");
    image.alt = this.alt;
    image.title = this.title;
    image.loading = "lazy";
    image.referrerPolicy = "no-referrer";
    image.addEventListener("error", () => {
      const fallback = view.dom.ownerDocument.createElement("span");
      fallback.className = "lm-image-fallback";
      fallback.textContent = this.alt || "图片无法加载";
      image.replaceWith(fallback);
    }, { once: true });
    image.src = this.url;
    wrapper.append(image);
    return wrapper;
  }
}
function decodeInline(value: string): string {
  return value.replace(/\\([!"#$%&'()*+,\-./:;<=>?@[\]\\^_`{|}~])/g, "$1");
}
/** 预览不请求相对资源，原生资源服务接入后再扩展。 */
export function isPreviewImageUrl(url: string): boolean {
  return /^(?:https?:\/\/|data:image\/)/i.test(url) && isSafeMarkdownUrl(url, true);
}

/** 独立构造装饰，便于检查范围和 active line 行为。 */
export function buildLiveMarkdownDecorations(view: EditorView, options: LiveMarkdownOptions = {}): DecorationSet {
  const ranges: ReturnType<Decoration["range"]>[] = [];
  const decoratedLines = new Set<string>();
  const neededImages = new Set<string>();
  const replacements: { from: number; to: number }[] = [];
  const active = (from: number, to: number) => options.showActiveSyntax !== false && view.hasFocus && view.state.selection.ranges.some(selection => {
    const start = view.state.doc.lineAt(selection.from).from;
    const end = view.state.doc.lineAt(selection.to).to;
    return from <= end && to >= start;
  });
  const lineStyle = (position: number, className: string) => {
    const from = view.state.doc.lineAt(position).from;
    const key = `${from}:${className}`;
    if (decoratedLines.has(key)) return;
    decoratedLines.add(key);
    ranges.push(Decoration.line({ class: className }).range(from));
  };
  const replace = (from: number, to: number, widget?: WidgetType) => {
    // 父节点替换后不再遍历子节点，并兜底避免任务框与列表前缀重叠。
    if (to <= from || replacements.some(range => from < range.to && to > range.from)) return;
    replacements.push({ from, to });
    ranges.push(Decoration.replace(widget ? { widget } : {}).range(from, to));
  };
  const firstVisible = view.visibleRanges[0];
  const lastVisible = view.visibleRanges.at(-1);
  if (firstVisible && lastVisible) {
    // 一次遍历覆盖可见区间，避免跨折叠区域的父节点被重复处理或漏掉后续子节点。
    syntaxTree(view.state).iterate({ from: firstVisible.from, to: lastVisible.to, enter(node) {
      const name = node.name;
      // 只排除实际替换的范围，编辑源码时仍保留原来的行装饰。
      if (name !== "Document" && name !== "Paragraph" && view.state.facet(liveBlockRanges).some(range => node.from < range.to && node.to > range.from)) return false;
      if (/^(?:ATX|Setext)Heading[1-6]$/.test(name)) {
        const first = view.state.doc.lineAt(node.from);
        const last = view.state.doc.lineAt(node.to);
        for (let number = first.number; number <= last.number; number++) lineStyle(view.state.doc.line(number).from, `md-h${name.at(-1)}`);
        if (!active(node.from, node.to)) {
          if (name.startsWith("ATX")) {
            const prefix = view.state.doc.sliceString(node.from, node.to).match(/^#{1,6}\s+/);
            if (prefix) replace(node.from, node.from + prefix[0].length);
          }
          for (const mark of node.node.getChildren("HeaderMark")) {
            if (name.startsWith("Setext")) lineStyle(mark.from, "lm-setext-delimiter");
            replace(mark.from, mark.to);
          }
        }
      }
      if (["StrongEmphasis", "Emphasis", "InlineCode", "Strikethrough"].includes(name)) {
        const marker = name === "InlineCode" ? "CodeMark" : name === "Strikethrough" ? "StrikethroughMark" : "EmphasisMark";
        const marks = node.node.getChildren(marker);
        const first = marks[0];
        const last = marks.at(-1);
        const className = name === "StrongEmphasis" ? "md-strong" : name === "Emphasis" ? "md-emphasis" : name === "Strikethrough" ? "md-strike" : "md-inline-code";
        if (first && last && first.to < last.from) ranges.push(Decoration.mark({ class: className }).range(first.to, last.from));
        if (!active(node.from, node.to)) marks.forEach(mark => replace(mark.from, mark.to));
        if (name === "InlineCode") return false;
      }
      if (name === "Link" || name === "Autolink") {
        const marks = node.node.getChildren("LinkMark");
        const opening = marks[0];
        const closing = marks.find(mark => view.state.doc.sliceString(mark.from, mark.to) === (name === "Link" ? "]" : ">"));
        if (opening && closing && opening.to < closing.from) {
          ranges.push(Decoration.mark({ class: "lm-markdown-link" }).range(opening.to, closing.from));
          if (!active(node.from, node.to)) {
            replace(opening.from, opening.to);
            replace(closing.from, node.to);
          }
        }
      }
      if (name === "Image") {
        if (!active(node.from, node.to)) {
          const urlNode = node.node.getChild("URL");
          const marks = node.node.getChildren("LinkMark");
          const closing = marks.find(mark => view.state.doc.sliceString(mark.from, mark.to) === "]");
          const titleNode = node.node.getChild("LinkTitle");
          const rawUrl = urlNode ? view.state.doc.sliceString(urlNode.from, urlNode.to) : "";
          const url = decodeInline(rawUrl.startsWith("<") ? rawUrl.slice(1, -1) : rawUrl);
          let previewUrl = isPreviewImageUrl(url) ? url : undefined;
          // 只解析安全的相对资源，危险协议和不支持的 data 类型不进入资源服务。
          if (!previewUrl && !/^(?:https?:|data:)/i.test(url) && isSafeMarkdownUrl(url, true)) {
            const resolved = options.resolveImage?.(url);
            if (resolved && isPreviewImageUrl(resolved)) previewUrl = resolved;
            if (!previewUrl && options.onImageNeeded && !neededImages.has(url)) {
              neededImages.add(url);
              // build 内不允许同步 dispatch；通知推迟到当前视图更新完成后。
              const notify = options.onImageNeeded;
              queueMicrotask(() => notify(url));
            }
          }
          if (previewUrl) {
            const alt = marks[0] && closing ? decodeInline(view.state.doc.sliceString(marks[0].to, closing.from)) : "";
            const title = titleNode ? decodeInline(view.state.doc.sliceString(titleNode.from + 1, titleNode.to - 1)) : "";
            replace(node.from, node.to, new ImagePreview(previewUrl, alt, title));
          }
        }
        return false;
      }
      if (name === "HorizontalRule") {
        if (!active(node.from, node.to)) replace(node.from, node.to, new Rule());
        return false;
      }
      if (name === "ListMark" && node.node.parent?.parent?.name === "BulletList" && !active(node.from, node.to)) {
        const item = node.node.parent;
        if (item.getChild("Task")?.getChild("TaskMarker")) replace(node.from, node.to);
        else replace(node.from, node.to, new Bullet());
      }
      if (name === "QuoteMark") {
        lineStyle(node.from, "md-quote");
        if (!active(node.from, node.to)) {
          const end = view.state.doc.sliceString(node.to, node.to + 1) === " " ? node.to + 1 : node.to;
          replace(node.from, end);
        }
      }
      if (name === "FencedCode" || name === "CodeBlock") {
        const first = view.state.doc.lineAt(node.from).number;
        const last = view.state.doc.lineAt(node.to).number;
        for (let number = first; number <= last; number++) {
          const line = view.state.doc.line(number);
          lineStyle(line.from, "md-code-line");
          if (name === "FencedCode" && (number === first || number === last)) lineStyle(line.from, "md-code-fence");
        }
        return false;
      }
      if (name === "TaskMarker" && !active(node.from, node.to)) {
        const checked = /x/i.test(view.state.doc.sliceString(node.from, node.to));
        replace(node.from, node.to, new TaskBox(checked, node.from));
        return false;
      }
    } });
  }
  return Decoration.set(ranges, true);
}

export function liveMarkdown(options: LiveMarkdownOptions = {}) {
  return ViewPlugin.fromClass(class {
    decorations: DecorationSet;
    private requestedImages = new Set<string>();
    private destroyed = false;
    private imageOptions: LiveMarkdownOptions = {
      ...options,
      onImageNeeded: options.onImageNeeded ? url => {
        if (this.destroyed || this.requestedImages.has(url)) return;
        this.requestedImages.add(url);
        options.onImageNeeded?.(url);
      } : undefined,
    };
    constructor(readonly view: EditorView) { this.decorations = buildLiveMarkdownDecorations(view, this.imageOptions); }
    update(update: ViewUpdate) {
      if (update.docChanged || update.selectionSet || update.viewportChanged || update.focusChanged || syntaxTree(update.startState) !== syntaxTree(update.state) || update.startState.facet(liveBlockRanges) !== update.state.facet(liveBlockRanges)) {
        this.decorations = buildLiveMarkdownDecorations(this.view, this.imageOptions);
      }
    }
    destroy() { this.destroyed = true; }
  }, { decorations: value => value.decorations });
}
