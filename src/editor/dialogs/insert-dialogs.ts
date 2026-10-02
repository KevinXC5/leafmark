import type { EditorView } from "@codemirror/view";
import { isolateHistory } from "@codemirror/commands";
import { parser, GFM } from "@lezer/markdown";
import type { SyntaxNode } from "@lezer/common";
import "./interaction-details.css";

export interface ImageBrowseResult {
  url: string;
  alt?: string;
  title?: string;
  /** 用于预览的安全图片地址，不替换写入 Markdown 的资源路径。 */
  previewUrl?: string;
}
export interface ImageDialogOptions {
  /** 原生端可返回经过服务转换的资源 URL；取消选择时返回 null。 */
  browse?: () => Promise<string | ImageBrowseResult | null | undefined>;
  /** 原生端可把已有的相对资源路径转换为可显示的图片地址。 */
  resolvePreview?: (url: string) => Promise<string | null | undefined>;
}
const markdownParser = parser.configure(GFM);
const rasterData = /^data:image\/(?:png|jpeg|gif|webp|avif|bmp|x-icon);base64,[a-z0-9+/=\s]+$/i;

/** 只允许明确支持的协议，相对路径和锚点无需协议。 */
export function isSafeMarkdownUrl(value: string, image = false): boolean {
  // Markdown 会解码 HTML 实体，协议校验必须使用同样的文本，防止隐藏冒号。
  const url = value.trim().replace(/&#(x[0-9a-f]+|[0-9]+);?/gi, (_, code: string) => {
    const point = code.toLowerCase().startsWith("x") ? parseInt(code.slice(1), 16) : parseInt(code, 10);
    return point > 0 && point <= 0x10ffff ? String.fromCodePoint(point) : "�";
  }).replace(/&colon;/gi, ":").replace(/&Tab;/g, "\t").replace(/&NewLine;/g, "\n");
  if (!url || /[\u0000-\u001f\u007f]/.test(url) || url.includes("\\")) return false;
  if (image && rasterData.test(url)) return true;
  const scheme = url.match(/^([^/?#]*):/);
  if (!scheme) return true;
  return (image ? /^(https?)$/i : /^(https?|mailto|tel)$/i).test(scheme[1]!);
}

function unescapeMarkdown(value: string): string {
  return value.replace(/\\([!"#$%&'()*+,\-./:;<=>?@[\]\\^_`{|}~])/g, "$1");
}
function escapeLabel(value: string): string {
  return value.replace(/\\/g, "\\\\").replace(/\[/g, "\\[").replace(/\]/g, "\\]").replace(/\r\n?|\n/g, " ");
}
function destination(value: string): string {
  // 尖括号包裹 URL，避免空格及括号破坏 Markdown 链接语法。
  return `<${value.trim().replace(/</g, "%3C").replace(/>/g, "%3E")}>`;
}

function findExisting(source: string, from: number, to: number, image: boolean) {
  const tree = markdownParser.parse(source);
  let result: SyntaxNode | null = null;
  tree.iterate({ from, to, enter(node) {
    if (node.name === (image ? "Image" : "Link") && node.from <= from && node.to >= to) result = node.node;
  } });
  // 选区可能恰好落在语法节点末尾，显式向左解析以保留编辑行为。
  if (!result && from === to) {
    let node: SyntaxNode | null = tree.resolveInner(from, -1);
    while (node) {
      if (node.name === (image ? "Image" : "Link")) { result = node; break; }
      node = node.parent;
    }
  }
  return result as SyntaxNode | null;
}

function openInsertDialog(view: EditorView, image: boolean, options: ImageDialogOptions = {}): void {
  const snapshot = view.state.doc;
  const selection = view.state.selection.main;
  const source = snapshot.toString();
  const existing = findExisting(source, selection.from, selection.to, image);
  const from = existing?.from ?? selection.from;
  const to = existing?.to ?? selection.to;
  let labelSource = snapshot.sliceString(selection.from, selection.to);
  let initialUrl = "";
  let initialTitle = "";
  if (existing) {
    const marks = existing.getChildren("LinkMark");
    const first = marks[0];
    const closing = marks.find(mark => source.slice(mark.from, mark.to) === "]");
    if (first && closing) labelSource = source.slice(first.to, closing.from);
    const url = existing.getChild("URL");
    const title = existing.getChild("LinkTitle");
    if (url) {
      initialUrl = source.slice(url.from, url.to);
      if (initialUrl.startsWith("<")) initialUrl = initialUrl.slice(1, -1);
      initialUrl = unescapeMarkdown(initialUrl);
    }
    if (title) initialTitle = unescapeMarkdown(source.slice(title.from + 1, title.to - 1));
  }
  const initialLabel = unescapeMarkdown(labelSource);
  const doc = view.dom.ownerDocument;
  const dialog = doc.createElement("dialog");
  dialog.className = `lm-dialog ${image ? "lm-image-dialog" : "lm-link-dialog"}`;
  const form = doc.createElement("form");
  const header = doc.createElement("header");
  header.className = "lm-detail-header";
  const heading = doc.createElement("h2");
  heading.textContent = existing ? (image ? "编辑图片" : "编辑链接") : (image ? "插入图片" : "插入链接");
  dialog.setAttribute("aria-label", heading.textContent);
  const shortcut = doc.createElement("span");
  shortcut.textContent = image ? "图片与路径" : "⌘K";
  header.append(heading, shortcut);
  const body = doc.createElement("div");
  body.className = "lm-detail-body";
  form.append(header, body);
  const field = (labelText: string, initial: string, parent: HTMLElement = body) => {
    const label = doc.createElement("label");
    label.className = "lm-detail-field";
    const caption = doc.createElement("span");
    caption.textContent = labelText;
    const input = doc.createElement("input");
    input.type = "text";
    input.value = initial;
    label.append(caption, input);
    parent.append(label);
    return input;
  };
  const tabs = doc.createElement("div");
  tabs.className = "lm-image-tabs";
  if (image) body.append(tabs);
  const url = field(image ? "本地路径" : "URL", initialUrl);
  url.required = true;
  url.placeholder = image ? "./assets/image.png" : "https://example.com";
  url.spellcheck = false;
  const urlLabel = url.parentElement!;
  const urlRow = doc.createElement("div");
  urlRow.className = image ? "lm-detail-input-row" : "lm-link-url";
  url.replaceWith(urlRow);
  if (!image) {
    const icon = doc.createElement("span");
    icon.className = "lm-url-icon";
    icon.textContent = "↗";
    icon.setAttribute("aria-hidden", "true");
    urlRow.append(icon);
  }
  urlRow.append(url);
  const preview = doc.createElement("div");
  preview.className = "lm-image-preview";
  const previewMessage = doc.createElement("span");
  previewMessage.textContent = "选择图片后显示预览";
  preview.append(previewMessage);
  const metadata = doc.createElement("div");
  metadata.className = "lm-image-metadata";
  const fileInfo = doc.createElement("span");
  const previewStatus = doc.createElement("span");
  previewStatus.setAttribute("role", "status");
  metadata.append(fileInfo, previewStatus);
  if (image) body.append(preview, metadata);
  // 链接默认只显示 URL；额外字段折叠，保留原有文本和标题编辑能力。
  const details = doc.createElement("details");
  details.className = "lm-link-details";
  const summary = doc.createElement("summary");
  summary.textContent = "文本与标题（可选）";
  details.append(summary);
  if (!image) body.append(details);
  const text = field(image ? "替代文本" : "链接文本", initialLabel, image ? body : details);
  const title = field("标题（可选）", initialTitle, details);
  if (image) {
    summary.textContent = "图片标题（可选）";
    body.append(details);
  }
  const error = doc.createElement("p");
  error.className = "lm-detail-error";
  error.setAttribute("role", "alert");
  let finished = false;
  let pending = false;
  let request = 0;
  let previewRequest = 0;
  let knownAsset: ImageBrowseResult | undefined;
  let selectedFile: File | undefined;
  const close = () => {
    if (finished) return;
    finished = true;
    request++;
    previewRequest++;
    dialog.close();
    dialog.remove();
    view.focus();
  };
  const actions = doc.createElement("div");
  actions.className = "lm-detail-actions";
  const save = doc.createElement("button");
  save.type = "submit";
  save.className = "primary";
  save.textContent = image ? (existing ? "应用修改" : "插入图片") : "应用 ↵";
  const markdownPreview = doc.createElement("p");
  markdownPreview.className = "lm-image-markdown";
  const markdown = () => {
    const label = existing && text.value === initialLabel ? labelSource : escapeLabel(text.value);
    const titleValue = title.value.replace(/\r\n?|\n/g, " ").replace(/\\/g, "\\\\").replace(/"/g, '\\"');
    return `${image ? "!" : ""}[${label}](${destination(url.value)}${titleValue ? ` "${titleValue}"` : ""})`;
  };
  const updateMarkdown = () => {
    markdownPreview.textContent = rasterData.test(url.value) ? `![${escapeLabel(text.value)}](<内嵌图片数据>)` : markdown();
  };
  const updatePreview = async () => {
    if (!image) return;
    const current = ++previewRequest;
    const address = url.value.trim();
    preview.replaceChildren(previewMessage);
    fileInfo.textContent = selectedFile ? `${selectedFile.name} · ${selectedFile.size < 1024 * 1024 ? `${Math.ceil(selectedFile.size / 1024)} KB` : `${(selectedFile.size / 1024 / 1024).toFixed(1)} MB`}` : address && !rasterData.test(address) ? address.split("/").at(-1)?.split(/[?#]/)[0] ?? "图片" : "";
    previewMessage.textContent = address ? "正在加载预览…" : "选择图片后显示预览";
    previewStatus.textContent = "";
    updateMarkdown();
    if (!isSafeMarkdownUrl(address, true)) { previewMessage.textContent = address ? "请输入安全的图片地址" : "选择图片后显示预览"; return; }
    try {
      const resolved = knownAsset?.url === address && knownAsset.previewUrl ? knownAsset.previewUrl : options.resolvePreview ? await options.resolvePreview(address) : address;
      if (finished || current !== previewRequest) return;
      if (!resolved || !isSafeMarkdownUrl(resolved, true)) { previewMessage.textContent = "此路径暂时无法预览，仍可插入"; return; }
      // 每次创建独立图片节点；过期的 load/error 事件不能覆盖新来源的状态。
      const picture = doc.createElement("img");
      picture.alt = text.value;
      picture.referrerPolicy = "no-referrer";
      picture.addEventListener("load", () => {
        if (finished || current !== previewRequest) return;
        preview.replaceChildren(picture);
        fileInfo.textContent += `${fileInfo.textContent ? " · " : ""}${picture.naturalWidth} × ${picture.naturalHeight}`;
        previewStatus.textContent = "预览已就绪";
      });
      picture.addEventListener("error", () => {
        if (finished || current !== previewRequest) return;
        previewMessage.textContent = "无法加载预览，请检查路径或图片地址";
        previewStatus.textContent = "预览不可用";
      });
      picture.src = resolved;
    } catch {
      if (!finished && current === previewRequest) previewMessage.textContent = "此路径暂时无法预览，仍可插入";
    }
  };
  const runBrowse = async (load: () => Promise<string | ImageBrowseResult | null | undefined>) => {
    const current = ++request;
    pending = true;
    save.disabled = true;
    error.textContent = "";
    try {
      const result = await load();
      if (finished || current !== request || !result) return;
      const asset = typeof result === "string" ? { url: result } : result;
      if (!isSafeMarkdownUrl(asset.url, true)) throw new Error("图片地址不安全或格式不受支持。");
      knownAsset = asset;
      url.value = asset.url;
      if (asset.alt !== undefined) text.value = asset.alt;
      if (asset.title !== undefined) title.value = asset.title;
      void updatePreview();
    } catch (cause) {
      if (!finished && current === request) error.textContent = cause instanceof Error ? cause.message : "读取图片失败。";
    } finally {
      if (!finished && current === request) { pending = false; save.disabled = false; }
    }
  };
  if (image) {
    const file = doc.createElement("input");
    file.type = "file";
    file.hidden = true;
    file.accept = "image/png,image/jpeg,image/gif,image/webp,image/avif,image/bmp,image/x-icon";
    file.setAttribute("aria-label", "图片文件");
    const browse = doc.createElement("button");
    browse.type = "button";
    browse.textContent = "浏览…";
    browse.addEventListener("click", () => {
      if (options.browse) { selectedFile = undefined; void runBrowse(options.browse); }
      else file.click();
    });
    urlRow.append(browse, file);
    file.addEventListener("change", () => {
      const selected = file.files?.[0];
      if (!selected) return;
      selectedFile = selected;
      void runBrowse(() => new Promise<ImageBrowseResult>((resolve, reject) => {
        if (!/^image\/(png|jpeg|gif|webp|avif|bmp|x-icon)$/i.test(selected.type)) { reject(new Error("请选择支持的图片文件。")); return; }
        const Reader = doc.defaultView?.FileReader ?? FileReader;
        const reader = new Reader();
        reader.addEventListener("load", () => resolve({ url: String(reader.result), alt: text.value || selected.name }));
        reader.addEventListener("error", () => reject(new Error("读取图片失败。")));
        reader.addEventListener("abort", () => reject(new Error("图片读取已取消。")));
        reader.readAsDataURL(selected);
      }));
    });
    tabs.setAttribute("role", "tablist");
    tabs.setAttribute("aria-label", "图片来源");
    const tabButtons: HTMLButtonElement[] = [];
    const setTab = (local: boolean) => {
      tabButtons.forEach((button, index) => { button.setAttribute("aria-selected", String(local === (index === 0))); button.tabIndex = local === (index === 0) ? 0 : -1; });
      urlLabel.querySelector("span")!.textContent = local ? "本地路径" : "图片 URL";
      url.placeholder = local ? "./assets/image.png" : "https://example.com/image.png";
      browse.hidden = !local;
    };
    for (const [index, label] of ["本地文件", "图片 URL"].entries()) {
      const tab = doc.createElement("button");
      tab.type = "button";
      tab.textContent = label;
      tab.setAttribute("role", "tab");
      tab.addEventListener("click", () => setTab(index === 0));
      tab.addEventListener("keydown", event => {
        if (["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) {
          event.preventDefault();
          const next = event.key === "Home" ? 0 : event.key === "End" ? 1 : 1 - index;
          setTab(next === 0);
          tabButtons[next]!.focus();
        }
      });
      tabButtons.push(tab);
      tabs.append(tab);
    }
    setTab(!/^https?:/i.test(initialUrl));
    const storage = doc.createElement("p");
    storage.className = "lm-image-storage";
    storage.textContent = options.browse ? "本地文件通过浏览导入，复制到文档附件目录并使用相对路径。" : "本地文件以内嵌图片数据保存；图片 URL 保留原始地址。";
    body.append(storage, markdownPreview);
    text.addEventListener("input", () => { const picture = preview.querySelector("img"); if (picture) picture.alt = text.value; updateMarkdown(); });
    title.addEventListener("input", updateMarkdown);
    url.addEventListener("input", () => { knownAsset = undefined; selectedFile = undefined; void updatePreview(); });
  } else if (existing) {
    const remove = doc.createElement("button");
    remove.type = "button";
    remove.className = "lm-link-remove";
    remove.textContent = "移除链接";
    remove.addEventListener("click", () => {
      if (view.state.doc !== snapshot) { error.textContent = "文档已变化，请关闭后重新打开对话框。"; return; }
      const insert = text.value === initialLabel ? labelSource : escapeLabel(text.value);
      view.dispatch({ changes: { from, to, insert }, selection: { anchor: from + insert.length }, annotations: isolateHistory.of("full"), userEvent: "input.link" });
      close();
    });
    actions.append(remove);
  }
  const cancel = doc.createElement("button");
  cancel.type = "button";
  cancel.textContent = "取消";
  cancel.addEventListener("click", close);
  actions.append(cancel, save);
  body.append(error, actions);
  if (!image) {
    const hint = doc.createElement("p");
    hint.className = "lm-detail-hint";
    hint.textContent = "Esc 取消 · Enter 应用到所选文字";
    body.append(hint);
  }
  dialog.append(form);
  form.addEventListener("submit", event => {
    event.preventDefault();
    if (pending) return;
    if (view.state.doc !== snapshot) { error.textContent = "文档已变化，请关闭后重新打开对话框。"; return; }
    if (!isSafeMarkdownUrl(url.value, image)) { error.textContent = "请输入安全的 URL 或相对路径。"; url.focus(); return; }
    const insert = markdown();
    // 只提交一次变更，并与前后的输入隔离，保证一次撤销恢复原始 Markdown。
    if (insert !== snapshot.sliceString(from, to)) {
      view.dispatch({ changes: { from, to, insert }, selection: { anchor: from + insert.length }, annotations: isolateHistory.of("full"), userEvent: image ? "input.image" : "input.link" });
    }
    close();
  });
  dialog.addEventListener("cancel", event => { event.preventDefault(); close(); });
  dialog.addEventListener("close", close);
  doc.body.append(dialog);
  dialog.showModal();
  if (!image) {
    // 链接弹窗靠近真实选区，并限制在视口内；编辑器不可测量时使用居中回退。
    try {
      const anchor = view.coordsAtPos(selection.head);
      if (anchor && doc.defaultView) {
        const { innerWidth, innerHeight } = doc.defaultView;
        const bounds = dialog.getBoundingClientRect();
        dialog.style.left = `${Math.max(12, Math.min(anchor.left, innerWidth - bounds.width - 12))}px`;
        dialog.style.top = `${Math.max(12, Math.min(anchor.bottom + 10, innerHeight - bounds.height - 12))}px`;
        dialog.classList.add("lm-link-anchored");
      }
    } catch { /* 无布局的测试环境使用居中回退。 */ }
  }
  void updatePreview();
  url.focus();
}

export function openLinkDialog(view: EditorView): void {
  openInsertDialog(view, false);
}
export function openImageDialog(view: EditorView, options: ImageDialogOptions = {}): void {
  openInsertDialog(view, true, options);
}
