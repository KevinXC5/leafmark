import MarkdownIt from "markdown-it";
import createDOMPurify, { type DOMPurify } from "dompurify";
import hljs from "highlight.js/lib/common";
import footnote from "markdown-it-footnote";
import mark from "markdown-it-mark";
import sub from "markdown-it-sub";
import sup from "markdown-it-sup";
import container from "markdown-it-container";
import katex from "katex";
import { calloutPattern, calloutType, parseWikiReference, wikiHref } from "./obsidian-syntax";
import { calloutIconHTML, calloutStyles } from "./callout";

// 使用原生 MathML 输出，导出无需外链字体或 KaTeX CSS。
function renderMath(source: string, displayMode: boolean): string {
  try {
    return katex.renderToString(source, { output: "mathml", displayMode, trust: false, throwOnError: true, maxExpand: 1000, maxSize: 20, strict: "error" });
  } catch {
    return `<code class="math-error">${escapeHTML(source)}</code>`;
  }
}

const safeImageData = /^data:image\/(?:png|jpeg|jpg|webp|gif);base64,[a-z\d+/]+={0,2}$/i;
function isSafeImageURL(value: string): boolean {
  return isSafeURL(value) || safeImageData.test(value);
}

export type ExportTheme = "light" | "dark" | "auto";

/** 允许网页、邮件、电话及相对地址；拒绝脚本、data 和自定义协议。 */
export function isSafeURL(value: string): boolean {
  const normalized = value.replace(/[\u0000- \u007f-\u009f]/g, "");
  const scheme = normalized.match(/^([a-z][a-z\d+.-]*):/i)?.[1]?.toLowerCase();
  return !scheme || ["http", "https", "mailto", "tel"].includes(scheme);
}

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: false,
  highlight(code, language) {
    // 只高亮明确声明且已注册的语言，未知语言由 markdown-it 进行转义。
    if (!language || !hljs.getLanguage(language)) return "";
    try { return hljs.highlight(code, { language, ignoreIllegals: true }).value; }
    catch { return ""; }
  },
});
markdown.validateLink = isSafeImageURL;
markdown.use(footnote).use(mark).use(sub).use(sup);
// 在标准链接规则之前消费完整引用，别名只作为文本输出。
markdown.inline.ruler.before("link", "leafmark_wiki", (state, silent) => {
  const reference = parseWikiReference(state.src, state.pos);
  if (!reference || state.pos + reference.length > state.posMax) return false;
  if (!silent) {
    const token = state.push("leafmark_wiki", "", 0);
    token.meta = { reference };
  }
  state.pos += reference.length;
  return true;
});
markdown.renderer.rules.leafmark_wiki = (tokens, index) => {
  const reference = tokens[index]!.meta!.reference as NonNullable<ReturnType<typeof parseWikiReference>>;
  const label = escapeHTML(reference.label), target = escapeHTML(reference.target);
  if (reference.embed) return `<span class="embed-placeholder" title="${target}">嵌入：${label}（暂未解析）</span>`;
  return `<a class="wiki-link" href="${escapeHTML(wikiHref(reference.target))}" title="${target}">${label}</a>`;
};
markdown.use(container, "details", {
  validate: (params: string) => /^details(?:\s|$)/.test(params.trim()),
  render: (tokens: { nesting: number; info: string }[], index: number) => tokens[index]!.nesting === 1
    ? `<details><summary>${escapeHTML(tokens[index]!.info.trim().slice(7).trim() || "详情")}</summary>\n`
    : "</details>\n",
});

// frontmatter 仅识别文档起始的 YAML 分隔块，不执行或解析元数据。
markdown.block.ruler.before("hr", "frontmatter", (state, start, end, silent) => {
  if (start !== 0 || state.bMarks[start] !== 0 || state.src.slice(0, state.eMarks[start]).trim() !== "---") return false;
  for (let line = start + 1; line < end; line++) {
    const text = state.src.slice(state.bMarks[line], state.eMarks[line]).trim();
    if (text !== "---" && text !== "...") continue;
    if (!silent) state.line = line + 1;
    return true;
  }
  return false;
});

// 支持独占一行的 $$ 围栏；内部交给 KaTeX，错误时转义为代码。
markdown.block.ruler.before("fence", "math_block", (state, start, end, silent) => {
  const lineText = (line: number) => state.src.slice(state.bMarks[line]! + state.tShift[line]!, state.eMarks[line]).trim();
  if (lineText(start) !== "$$") return false;
  let close = start + 1;
  while (close < end && lineText(close) !== "$$") close++;
  if (close === end) return false;
  if (silent) return true;
  const token = state.push("math_block", "", 0);
  token.content = state.getLines(start + 1, close, state.blkIndent, false);
  token.map = [start, close + 1];
  state.line = close + 1;
  return true;
});
markdown.inline.ruler.before("escape", "math_inline", (state, silent) => {
  const from = state.pos;
  if (state.src[from] !== "$" || state.src[from + 1] === "$" || /\s/.test(state.src[from + 1] ?? " ")) return false;
  let to = from + 1;
  while ((to = state.src.indexOf("$", to)) >= 0) {
    let slashes = 0;
    for (let index = to - 1; index > from && state.src[index] === "\\"; index--) slashes++;
    if (slashes % 2 === 0) break;
    to++;
  }
  if (to < 0 || to >= state.posMax || /\s/.test(state.src[to - 1]!) || /\d/.test(state.src[to + 1] ?? "") || state.src.slice(from + 1, to).includes("\n")) return false;
  if (!silent) state.push("math_inline", "", 0).content = state.src.slice(from + 1, to);
  state.pos = to + 1;
  return true;
});
markdown.renderer.rules.math_inline = (tokens, index) => renderMath(tokens[index]!.content, false);
markdown.renderer.rules.math_block = (tokens, index) => renderMath(tokens[index]!.content, true) + "\n";

// GitHub/Obsidian 风格引用提示块：> [!NOTE] 标题，后续仍按普通引用解析。
markdown.core.ruler.after("inline", "leafmark_callout", state => {
  for (let i = 0; i < state.tokens.length - 2; i++) {
    const opening = state.tokens[i]!;
    const inline = state.tokens[i + 2]!;
    if (opening.type !== "blockquote_open" || state.tokens[i + 1]?.type !== "paragraph_open" || inline.type !== "inline") continue;
    const first = inline.children?.[0];
    const match = first?.type === "text" ? first.content.match(calloutPattern) : null;
    if (!first || !match) continue;
    opening.attrJoin("class", `callout callout-${calloutType(match[1]!)}`);
    first.content = match[3] || match[1]!;
    const icon = new state.Token("html_inline", "", 0);
    icon.content = calloutIconHTML(match[1]!);
    const strongOpen = new state.Token("strong_open", "strong", 1);
    strongOpen.attrSet("class", "callout-title");
    const strongClose = new state.Token("strong_close", "strong", -1);
    inline.children!.splice(0, 1, icon, strongOpen, first, strongClose);
    // 同一段落中的正文与标题紧凑衔接，多段落仍保留各自结构。
    const separator = inline.children![4];
    if (separator?.type === "softbreak") {
      separator.type = "html_inline";
      separator.content = '<span class="callout-separator"> · </span>';
    }
  }
});

// 同文档 wiki 标题和块引用使用原文锚点，与编码后的 href 对应。
markdown.core.ruler.after("inline", "leafmark_wiki_anchors", state => {
  const targets = new Set<string>();
  for (const token of state.tokens) for (const child of token.children ?? []) {
    if (child.type === "leafmark_wiki") {
      const reference = child.meta!.reference as NonNullable<ReturnType<typeof parseWikiReference>>;
      if (!reference.embed && reference.target.startsWith("#")) targets.add(reference.target.slice(1));
    }
  }
  for (let index = 0; index < state.tokens.length; index++) {
    const token = state.tokens[index]!;
    const inline = state.tokens[index + 1];
    if (token.type === "heading_open" && inline?.type === "inline") {
      const text = (inline.children ?? []).filter(child => ["text", "code_inline"].includes(child.type)).map(child => child.content).join("");
      if (targets.has(text)) token.attrSet("id", text);
    }
    if (token.type === "paragraph_open" && inline?.type === "inline") {
      const last = inline.children?.at(-1);
      const block = last?.type === "text" ? last.content.match(/(?:^|[ \t])\^([A-Za-z\d-]+)[ \t]*$/) : null;
      if (last && block && targets.has("^" + block[1])) {
        token.attrSet("id", "^" + block[1]);
        last.content = last.content.slice(0, block.index);
      }
    }
  }
});

// data URI 仅用于图片；相同地址作为链接时移除 href。
markdown.core.ruler.after("inline", "leafmark_safe_links", state => {
  for (const token of state.tokens) {
    for (const child of token.children ?? []) {
      if (child.type === "link_open" && !isSafeURL(String(child.attrGet("href") ?? ""))) {
        child.attrs = (child.attrs ?? []).filter(([name]) => name !== "href");
      }
    }
  }
});

// markdown-it 原生支持表格和删除线；任务框由受控 token 生成，不开放原始 HTML。
markdown.core.ruler.after("inline", "leafmark_tasks", state => {
  for (let i = 2; i < state.tokens.length; i++) {
    const token = state.tokens[i]!;
    if (token.type !== "inline" || state.tokens[i - 1]?.type !== "paragraph_open" || state.tokens[i - 2]?.type !== "list_item_open") continue;
    const first = token.children?.[0];
    const task = first?.type === "text" ? first.content.match(/^\[([ xX])\][ \t]+/) : null;
    if (!first || !task) continue;
    first.content = first.content.slice(task[0].length);
    const checkbox = new state.Token("html_inline", "", 0);
    const checked = task[1]!.toLowerCase() === "x";
    checkbox.content = `<input class="task-list-checkbox" type="checkbox" disabled${checked ? " checked" : ""} aria-label="${checked ? "已完成" : "未完成"}"> `;
    token.children!.unshift(checkbox);
    state.tokens[i - 2]!.attrJoin("class", "task-list-item");
  }
});

let purifier: DOMPurify | undefined;
function getPurifier(): DOMPurify {
  if (purifier) return purifier;
  if (typeof window === "undefined" || !window.document) {
    throw new Error("Markdown 安全渲染需要浏览器 DOM 环境。");
  }
  purifier = createDOMPurify(window);
  // 对解析后的属性再次检查，覆盖实体解码、控制字符及插件生成内容。
  purifier.addHook("uponSanitizeAttribute", (_node, data) => {
    if (data.attrName === "href" && !isSafeURL(data.attrValue)) data.keepAttr = false;
    if (data.attrName === "src" && !isSafeImageURL(data.attrValue)) data.keepAttr = false;
  });
  return purifier;
}

const sanitizeOptions = {
  ALLOWED_TAGS: ["svg", "path", "circle", "line", "polyline", "rect", "p", "br", "hr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "ul", "ol", "li", "strong", "em", "s", "a", "img", "pre", "code", "span", "table", "thead", "tbody", "tr", "th", "td", "input", "mark", "sub", "sup", "section", "details", "summary", "math", "semantics", "annotation", "mrow", "mi", "mn", "mo", "mtext", "mspace", "msup", "msub", "msubsup", "mfrac", "msqrt", "mroot", "mover", "munder", "munderover", "mtable", "mtr", "mtd", "menclose", "mstyle", "mpadded", "mphantom"],
  ALLOWED_ATTR: ["viewBox", "fill", "stroke", "stroke-width", "stroke-linecap", "stroke-linejoin", "d", "cx", "cy", "r", "x", "y", "x1", "x2", "y1", "y2", "rx", "ry", "points", "aria-hidden", "href", "src", "alt", "title", "class", "id", "start", "align", "type", "checked", "disabled", "aria-label", "xmlns", "display", "encoding", "mathvariant", "mathsize", "mathcolor", "displaystyle", "scriptlevel", "stretchy", "fence", "separator", "lspace", "rspace", "minsize", "maxsize", "accent", "accentunder", "columnalign", "columnspacing", "rowspacing", "columnlines", "rowlines", "width", "height", "depth", "voffset", "notation", "linethickness"],
  ALLOW_DATA_ATTR: false,
  ALLOW_ARIA_ATTR: false,
  // 表格对齐使用 align 属性，避免放开任意内联 style。
  FORBID_ATTR: ["style"],
};

/** 返回可插入正文容器的安全 HTML 片段；需要浏览器 DOM。 */
export function renderMarkdown(content: string): string {
  return getPurifier().sanitize(markdown.render(content), sanitizeOptions);
}

/** 只渲染行内语法（加粗、链接、行内代码、高亮、公式等），供表格单元格这类单行内容使用。 */
export function renderInlineMarkdown(content: string): string {
  return getPurifier().sanitize(markdown.renderInline(content), sanitizeOptions);
}

// markdown-it 默认以 style 输出表格对齐，转成安全净化允许的 align。
for (const name of ["th_open", "td_open"]) {
  markdown.renderer.rules[name] = (tokens, index, options, _env, renderer) => {
    const token = tokens[index]!;
    const align = String(token.attrGet("style") ?? "").match(/^text-align:(left|center|right)$/)?.[1];
    if (align) {
      token.attrs = (token.attrs ?? []).filter(([key]) => key !== "style");
      token.attrSet("align", align);
    }
    return renderer.renderToken(tokens, index, options);
  };
}

const escapeHTML = (text: string) => text.replace(/[&<>"']/g, char => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]!);
const exportStyles = `
:root{color-scheme:light;--bg:#fff;--fg:#24292f;--muted:#57606a;--line:#d0d7de;--code:#f6f8fa;--link:#0969da;--syntax:#8250df}
:root[data-theme="dark"]{color-scheme:dark;--bg:#161b22;--fg:#e6edf3;--muted:#9da7b3;--line:#444c56;--code:#21262d;--link:#79c0ff;--syntax:#d2a8ff}
@media(prefers-color-scheme:dark){:root[data-theme="auto"]{color-scheme:dark;--bg:#161b22;--fg:#e6edf3;--muted:#9da7b3;--line:#444c56;--code:#21262d;--link:#79c0ff;--syntax:#d2a8ff}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.75 system-ui,-apple-system,"Segoe UI",sans-serif}main{max-width:860px;margin:0 auto;padding:48px 32px;overflow-wrap:anywhere}h1,h2,h3,h4,h5,h6{line-height:1.3;margin:1.5em 0 .6em}h1,h2{padding-bottom:.3em;border-bottom:1px solid var(--line)}a{color:var(--link)}blockquote{margin:1em 0;padding:0 1em;border-left:4px solid var(--line);color:var(--muted)}pre,code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;background:var(--code);border-radius:5px}code{padding:.15em .35em;font-size:.9em}pre{padding:18px;overflow:auto;line-height:1.5}pre code{padding:0;background:none}img{max-width:100%;height:auto}table{border-collapse:separate;border-spacing:0;display:block;width:fit-content;max-width:100%;overflow:auto;margin:1em 0;border:1px solid var(--line);border-radius:5px}th,td{padding:8px 14px;border:0;border-right:1px solid var(--line);border-bottom:1px solid var(--line)}tr>:last-child{border-right:0}table>:last-child>tr:last-child>*{border-bottom:0}th{background:var(--code)}thead tr:first-child>:first-child{border-top-left-radius:4px}thead tr:first-child>:last-child{border-top-right-radius:4px}hr{border:0;border-top:1px solid var(--line);margin:2em 0}mark{background:#fff0a6;color:#24292f}.callout{border-left-color:var(--link);background:var(--code);padding:12px 18px;border-radius:5px}details{border:1px solid var(--line);border-radius:5px;padding:12px 18px;margin:1em 0}summary{cursor:pointer;font-weight:600}.footnotes{font-size:.9em;color:var(--muted)}math[display="block"]{display:block;overflow-x:auto;margin:1em 0}.task-list-item{list-style:none}.task-list-checkbox{margin-right:.4em}.hljs-keyword,.hljs-selector-tag,.hljs-literal{color:var(--syntax)}.hljs-string,.hljs-title,.hljs-number,.hljs-attr{color:var(--link)}.hljs-comment,.hljs-quote{color:var(--muted);font-style:italic}
${calloutStyles}
:root[data-theme="dark"]{--callout-bg:#2d2f33;--callout-ink:#e5e6e9;--callout-accent:#d99a7c}
@media(prefers-color-scheme:dark){:root[data-theme="auto"]{--callout-bg:#2d2f33;--callout-ink:#e5e6e9;--callout-accent:#d99a7c}}
@media print{:root,:root[data-theme]{color-scheme:light;--bg:#fff;--fg:#000;--muted:#444;--line:#bbb;--code:#f5f5f5;--link:#000;--syntax:#333}body{font-size:11pt}main{max-width:none;padding:0}pre{white-space:pre-wrap;overflow:visible}table{display:table;overflow:visible;width:100%}thead{display:table-header-group}tr,img{break-inside:avoid}h1,h2,h3,h4,h5,h6{break-after:avoid}a{text-decoration:underline}@page{margin:18mm}}
`;

/** 返回内嵌样式的完整 HTML 文档；调用方负责保存或下载。图片保留安全原始 URL。 */
export function exportHTML(name: string, content: string, theme: ExportTheme = "light"): string {
  const safeTheme = ["light", "dark", "auto"].includes(theme) ? theme : "light";
  const title = escapeHTML(name);
  return `<!doctype html>\n<html lang="zh-CN" data-theme="${safeTheme}">\n<head>\n<meta charset="UTF-8">\n<meta name="viewport" content="width=device-width, initial-scale=1">\n<title>${title}</title>\n<style>${exportStyles}</style>\n</head>\n<body><main>${renderMarkdown(content)}</main></body>\n</html>`;
}
