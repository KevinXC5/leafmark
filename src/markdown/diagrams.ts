import createDOMPurify from "dompurify";
import { Check, Copy, createElement } from "lucide";
import type { MermaidConfig } from "mermaid";

export const DIAGRAM_LIMITS = Object.freeze({ characters: 12_000, lines: 200, tokens: 1_200, edges: 200 });

/** 保守限制词元和连线数，同时覆盖流程图以外语法；不尝试完整解析所有图类型。 */
export function validateDiagramSource(source: string): string | null {
  if (!source.trim()) return "图形源码为空。";
  if (source.length > DIAGRAM_LIMITS.characters) return "图形源码过长，请拆分为多个图形。";
  if (source.split(/\r?\n/).length > DIAGRAM_LIMITS.lines) return "图形行数过多，请拆分为多个图形。";
  if ((source.match(/[\p{L}\p{N}_]+|[^\s\p{L}\p{N}_]/gu) ?? []).length > DIAGRAM_LIMITS.tokens) return "图形节点或语法过多，请简化图形。";
  if ((source.match(/-->|---|==>|-\.->|->>|-->>|\.\.|--|==/g) ?? []).length > DIAGRAM_LIMITS.edges) return "图形连线过多，请简化图形。";
  // 禁止文档覆盖运行时配置和远程资源，保持模块的安全与消耗边界。
  if (/%%\s*\{|^\s*---\s*$|@\{[^}]*\b(?:img|icon)\s*:|^\s*(?:click|link)\s|^\s*(?:classDef|style)\s/im.test(source)) return "图形暂不支持配置指令、交互链接、外部图片或自定义样式。";
  return null;
}

/** 图形配色取自应用的纸色与墨色（见 src/app/style.css），节点、连线和标注与正文同一色调。 */
export const DIAGRAM_PALETTES = Object.freeze({
  light: Object.freeze({ paper: "#faf9f5", ink: "#47423c", heading: "#302e2b", node: "#f3e8dd", nodeEdge: "#d6b19a", line: "#9b8b7b", group: "#f4f0e8", groupEdge: "#e0d6c8", note: "#f1ebe3", noteEdge: "#dccdbd" }),
  dark: Object.freeze({ paper: "#242527", ink: "#cbcdd3", heading: "#e8e9ec", node: "#343b44", nodeEdge: "#6c7c8d", line: "#9da7b3", group: "#2a2c30", groupEdge: "#43464d", note: "#2d2f33", noteEdge: "#4a4e56" }),
});

export function diagramConfig(root: HTMLElement): MermaidConfig {
  const theme = root.dataset.theme;
  const dark = theme === "dark" || ((theme === "auto" || !theme) && root.ownerDocument.defaultView?.matchMedia?.("(prefers-color-scheme: dark)").matches);
  return {
    startOnLoad: false,
    securityLevel: "strict",
    // base 是 Mermaid 唯一可定制的主题，其余颜色由下列变量派生。
    theme: "base",
    themeVariables: diagramThemeVariables(dark ? "dark" : "light"),
    htmlLabels: false,
    flowchart: { htmlLabels: false },
    maxTextSize: DIAGRAM_LIMITS.characters,
    maxEdges: DIAGRAM_LIMITS.edges,
    suppressErrorRendering: true,
    fontFamily: "system-ui, sans-serif",
  };
}

function diagramThemeVariables(mode: keyof typeof DIAGRAM_PALETTES): Record<string, string | boolean> {
  const c = DIAGRAM_PALETTES[mode];
  return {
    darkMode: mode === "dark",
    background: c.paper,
    textColor: c.ink, titleColor: c.heading, lineColor: c.line,
    primaryColor: c.node, primaryTextColor: c.ink, primaryBorderColor: c.nodeEdge,
    secondaryColor: c.group, secondaryTextColor: c.ink, secondaryBorderColor: c.groupEdge,
    tertiaryColor: c.group, tertiaryTextColor: c.ink, tertiaryBorderColor: c.groupEdge,
    mainBkg: c.node, nodeBorder: c.nodeEdge, nodeTextColor: c.ink,
    clusterBkg: c.group, clusterBorder: c.groupEdge, edgeLabelBackground: c.paper,
    noteBkgColor: c.note, noteBorderColor: c.noteEdge, noteTextColor: c.ink,
    actorBkg: c.node, actorBorder: c.nodeEdge, actorTextColor: c.ink, actorLineColor: c.line,
    signalColor: c.line, signalTextColor: c.ink, labelBoxBkgColor: c.group, labelBoxBorderColor: c.groupEdge, labelTextColor: c.ink,
    activationBkgColor: c.group, activationBorderColor: c.nodeEdge,
  };
}

/** SVG 只保留图形内容；引用限定在当前 SVG 内，拒绝外部资源和可执行节点。 */
export function sanitizeDiagramSVG(svg: string, document: Document): string {
  const window = document.defaultView;
  if (!window) throw new Error("图形渲染需要浏览器 DOM 环境。");
  const purifier = createDOMPurify(window);
  purifier.addHook("uponSanitizeAttribute", (_node, data) => {
    if (["href", "xlink:href"].includes(data.attrName) && !/^#[\w:.-]+$/.test(data.attrValue)) data.keepAttr = false;
    if (/\\|@import|(?:url\s*\(\s*['"]?(?!#))|expression\s*\(/i.test(data.attrValue)) data.keepAttr = false;
  });
  const cleaned = purifier.sanitize(svg, {
    USE_PROFILES: { svg: true, svgFilters: true },
    ADD_TAGS: ["use"],
    FORBID_TAGS: ["foreignObject", "script", "iframe", "image", "a", "animate", "animateMotion", "animateTransform", "set"],
    FORBID_ATTR: ["onload", "onclick"],
    ALLOW_DATA_ATTR: false,
  });
  const template = document.createElement("template");
  template.innerHTML = cleaned;
  const result = template.content.querySelector("svg");
  if (!result) throw new Error("未生成有效的 SVG 图形。");
  for (const style of result.querySelectorAll("style")) {
    if (/\\|@import|(?:url\s*\(\s*['"]?(?!#))|expression\s*\(/i.test(style.textContent ?? "")) style.remove();
  }
  return result.outerHTML;
}

let serial = 0;
let queue: Promise<void> = Promise.resolve();
const pending = new WeakSet<HTMLElement>();

export interface DiagramOptions {
  /** 提供后，图形右上角悬浮显示复制源码的图标；导出等静态场景不传。 */
  copyText?: (text: string) => Promise<void>;
}

/** 悬浮在图形右上角的复制图标，正文中不常驻任何控件。 */
function diagramCopyButton(doc: Document, source: string, copyText: NonNullable<DiagramOptions["copyText"]>): HTMLButtonElement {
  const button = doc.createElement("button");
  button.type = "button";
  button.className = "diagram-copy";
  const show = (label: string, icon: typeof Copy) => {
    button.title = label; button.setAttribute("aria-label", label);
    button.replaceChildren(createElement(icon, { width: 14, height: 14, "aria-hidden": "true" }));
  };
  show("复制图形源码", Copy);
  let timer: ReturnType<typeof setTimeout> | undefined;
  button.addEventListener("click", async () => {
    clearTimeout(timer);
    try { await copyText(source); show("已复制", Check); }
    catch { show("复制失败，请切回编辑后手动复制", Copy); }
    timer = setTimeout(() => show("复制图形源码", Copy), 1600);
  });
  return button;
}

/** 查找 Mermaid 代码块，按序动态加载、渲染并替换为图形；失败保留源码供选择复制。 */
export async function renderDiagrams(container: HTMLElement, options: DiagramOptions = {}): Promise<void> {
  const blocks = [...container.querySelectorAll<HTMLElement>("pre code.language-mermaid")];
  const jobs: Promise<void>[] = [];
  for (const code of blocks) {
    const pre = code.parentElement;
    if (!pre || pending.has(pre) || pre.dataset.diagramError || pre.dataset.diagramRendered) continue;
    pending.add(pre);
    const source = code.textContent ?? "";
    const job = queue.then(async () => {
      let staging: HTMLDivElement | undefined;
      try {
        const validation = validateDiagramSource(source);
        if (validation) throw new Error(validation);
        if (!container.contains(pre) || code.textContent !== source) return;
        const mermaid = (await import("mermaid")).default;
        mermaid.initialize(diagramConfig(container.ownerDocument.documentElement));
        // 独立挂载点方便成功或失败后完整清理 Mermaid 的临时 DOM。
        staging = container.ownerDocument.createElement("div");
        staging.style.position = "absolute";
        staging.style.left = "-100000px";
        staging.style.visibility = "hidden";
        container.ownerDocument.body.append(staging);
        const id = `leafmark-diagram-${Date.now().toString(36)}-${++serial}`;
        const { svg } = await mermaid.render(id, source, staging);
        if (!container.contains(pre) || code.textContent !== source) return;
        const figure = container.ownerDocument.createElement("figure");
        figure.className = "mermaid-diagram";
        figure.setAttribute("aria-label", "Mermaid 图形");
        figure.innerHTML = sanitizeDiagramSVG(svg, container.ownerDocument);
        figure.style.margin = "1em 0";
        figure.style.overflowX = "auto";
        const image = figure.querySelector("svg")!;
        // 图形最多按自身尺寸显示并居中，宽于正文时等比缩小；小图不随容器放大。
        const naturalWidth = Number(image.getAttribute("viewBox")?.trim().split(/[\s,]+/)[2]);
        image.style.maxWidth = naturalWidth > 0 ? `min(100%, ${Math.ceil(naturalWidth)}px)` : "100%";
        image.style.height = "auto";
        image.style.margin = "0 auto";
        if (options.copyText) figure.append(diagramCopyButton(container.ownerDocument, source, options.copyText));
        pre.replaceWith(figure);
      } catch (error) {
        if (!container.contains(pre)) return;
        const message = container.ownerDocument.createElement("p");
        message.className = "mermaid-error";
        message.setAttribute("role", "status");
        message.textContent = `图形渲染失败：${validateDiagramSource(source) ?? "请检查 Mermaid 语法。"} 源码已保留，可选择复制。`;
        // 不插入解析器的原始错误 HTML，避免错误路径引入可执行内容。
        pre.before(message);
        pre.dataset.diagramError = "true";
      } finally {
        staging?.remove();
        pending.delete(pre);
      }
    });
    queue = job.catch(() => {});
    jobs.push(job);
  }
  await Promise.all(jobs);
}
