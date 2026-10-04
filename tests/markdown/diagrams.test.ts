import { describe, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { fileURLToPath } from "node:url";
// Mermaid 自身的 DOMPurify 在模块加载时检查浏览器环境。
Object.defineProperty(globalThis, "window", { value: new JSDOM("").window, configurable: true, writable: true });
const { DIAGRAM_LIMITS, DIAGRAM_PALETTES, diagramConfig, renderDiagrams, sanitizeDiagramSVG, validateDiagramSource } = await import("../../src/markdown/diagrams");

describe("Mermaid 安全和资源边界", () => {
  test("主题与严格配置", () => {
    const root = new JSDOM("").window.document.documentElement;
    root.dataset.theme = "dark";
    const config = diagramConfig(root);
    expect(config.theme).toBe("base");
    expect(config.themeVariables).toMatchObject({ darkMode: true, background: DIAGRAM_PALETTES.dark.paper, primaryColor: DIAGRAM_PALETTES.dark.node, lineColor: DIAGRAM_PALETTES.dark.line });
    expect(config.securityLevel).toBe("strict");
    expect(config.startOnLoad).toBe(false);
    expect(config.htmlLabels).toBe(false);
    expect(config.maxEdges).toBe(DIAGRAM_LIMITS.edges);
    root.dataset.theme = "light";
    expect(diagramConfig(root).themeVariables).toMatchObject({ darkMode: false, background: DIAGRAM_PALETTES.light.paper, primaryColor: DIAGRAM_PALETTES.light.node, primaryTextColor: DIAGRAM_PALETTES.light.ink });
  });
  test("限制长度、行数、词元、连线及配置覆盖", () => {
    expect(validateDiagramSource("graph TD; A-->B")).toBeNull();
    for (const source of ["", "x".repeat(12_001), "A\n".repeat(201), "A ".repeat(1201), "A-->B;".repeat(201), '%%{init: {"securityLevel":"loose"}}%%\ngraph TD', "---\nconfig:\n---\ngraph TD", "graph TD\nclick A href \"https://example.com\"", "graph TD\nA@{ img: \"https://example.com/a.png\" }"]) {
      expect(validateDiagramSource(source)).not.toBeNull();
    }
  });
  test("净化 SVG，拒绝脚本、外部资源及 CSS URL", () => {
    const document = new JSDOM("").window.document;
    const cleaned = sanitizeDiagramSVG('<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(1)</script><foreignObject><div>内容</div></foreignObject><image href="https://example.com/x"/><style>@import "https://example.com/x";</style><rect width="10" height="10" style="fill:url(https://example.com/x)"/><use href="#local"/><use href="https://example.com/x"/></svg>', document);
    const host = document.createElement("div");
    host.innerHTML = cleaned;
    expect(host.querySelector("svg")).not.toBeNull();
    expect(host.querySelector("rect")).not.toBeNull();
    expect(host.querySelector("script,foreignObject,image,style,[onload]")).toBeNull();
    expect(host.querySelector('use[href="#local"]')).not.toBeNull();
    expect(cleaned).not.toContain("https://example.com");
  });
  test("实际 Mermaid 渲染、源码保留与重复调用", () => {
    // 独立进程避免其他测试的 DOMPurify 缓存绑定到不同 window。
    const script = `
      import { JSDOM } from "jsdom";
      const w = new JSDOM("<html><body></body></html>").window;
      Object.assign(globalThis, { window:w, document:w.document, DOMParser:w.DOMParser, HTMLElement:w.HTMLElement, SVGElement:w.SVGElement, CSSStyleSheet:w.CSSStyleSheet });
      Object.defineProperty(w.SVGElement.prototype, "getBBox", { value: () => ({x:0,y:0,width:100,height:20}) });
      Object.defineProperty(w.SVGElement.prototype, "getComputedTextLength", { value: () => 40 });
      Object.defineProperty(w.CSSStyleSheet.prototype, "replaceSync", { value(text) {
        for (const rule of text.split("}")) if (rule.trim()) try { this.insertRule(rule + "}", this.cssRules.length); } catch {}
      }});
      const { renderDiagrams } = await import("./src/markdown/diagrams");
      const container = w.document.createElement("div");
      container.innerHTML = '<pre><code class="language-mermaid">graph TD; A-->B</code></pre>';
      w.document.body.append(container);
      let copied = "";
      await renderDiagrams(container, { copyText: async text => { copied = text; } });
      if (!container.querySelector("figure svg")) throw new Error(container.textContent);
      if (container.querySelector("details,summary,pre")) throw new Error("正文中残留源码区");
      const copy = container.querySelector("figure button.diagram-copy");
      if (copy?.getAttribute("aria-label") !== "复制图形源码") throw new Error("缺少悬浮复制入口");
      copy.click(); await new Promise(resolve => setTimeout(resolve, 0));
      if (copied !== "graph TD; A-->B" || copy.getAttribute("aria-label") !== "已复制") throw new Error("未复制图形源码");
      if (container.querySelector("script,foreignObject,image")) throw new Error("存在危险节点");
      await renderDiagrams(container);
      if (container.querySelectorAll("figure").length !== 1 || w.document.body.children.length !== 1) throw new Error("重复渲染或临时节点未清理");
    `;
    const result = Bun.spawnSync([process.execPath, "-e", script], { cwd: fileURLToPath(new URL("../..", import.meta.url)) });
    expect(result.exitCode, result.stderr.toString()).toBe(0);
  });
  test("无效输入保留源码并显示中文，重复调用不重复提示", async () => {
    const document = new JSDOM("").window.document;
    const container = document.createElement("div");
    container.innerHTML = '<pre><code class="language-mermaid"></code></pre>';
    container.querySelector("code")!.textContent = "x".repeat(12_001);
    await renderDiagrams(container);
    expect(container.querySelector(".mermaid-error")?.textContent).toContain("源码已保留");
    expect(container.querySelector("code")?.textContent).toHaveLength(12_001);
    await renderDiagrams(container);
    expect(container.querySelectorAll(".mermaid-error").length).toBe(1);
  });
});
