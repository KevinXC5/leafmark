import createDOMPurify from "dompurify";
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

export function diagramConfig(root: HTMLElement): MermaidConfig {
  const theme = root.dataset.theme;
  const dark = theme === "dark" || ((theme === "auto" || !theme) && root.ownerDocument.defaultView?.matchMedia?.("(prefers-color-scheme: dark)").matches);
  return {
    startOnLoad: false,
    securityLevel: "strict",
    theme: dark ? "dark" : "default",
    htmlLabels: false,
    flowchart: { htmlLabels: false },
    maxTextSize: DIAGRAM_LIMITS.characters,
    maxEdges: DIAGRAM_LIMITS.edges,
    suppressErrorRendering: true,
    fontFamily: "system-ui, sans-serif",
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

/** 查找 Mermaid 代码块，按序动态加载、渲染并替换；失败保留源码供选择复制。 */
export async function renderDiagrams(container: HTMLElement): Promise<void> {
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
        image.style.maxWidth = "100%";
        image.style.height = "auto";
        // 保留源码折叠区，便于选择复制；序列化正文也会保留 SVG 与源码。
        const details = container.ownerDocument.createElement("details");
        const summary = container.ownerDocument.createElement("summary");
        summary.textContent = "图形源码（可选择复制）";
        details.append(summary);
        pre.replaceWith(figure);
        details.append(pre);
        figure.append(details);
        pre.dataset.diagramRendered = "true";
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
