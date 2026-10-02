import { beforeAll, describe, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { exportHTML, isSafeURL, renderMarkdown } from "../../src/markdown/render-markdown";

beforeAll(() => {
  // 仅为净化测试提供 DOM，生产模块仍使用浏览器自身的 DOM。
  Object.defineProperty(globalThis, "window", { value: new JSDOM("").window, configurable: true });
});

function parse(html: string) {
  const window = new JSDOM("").window;
  window.document.body.innerHTML = html;
  return window.document.body;
}

describe("Markdown 渲染", () => {
  test("表格、对齐、删除线、任务列表和链接", () => {
    const html = renderMarkdown("| A | B |\n| :--- | ---: |\n| 一 | 二 |\n\n- [ ] 未完成\n- [x] 已完成\n\n~~删除~~ [链接](https://example.com) https://example.com");
    const body = parse(html);
    expect(body.querySelectorAll("table").length).toBe(1);
    expect(body.querySelector("th")?.getAttribute("align")).toBe("left");
    expect(body.querySelectorAll("input").length).toBe(2);
    expect(body.querySelectorAll("input[disabled]").length).toBe(2);
    expect(body.querySelectorAll("input[checked]").length).toBe(1);
    expect(body.querySelector("s")?.textContent).toBe("删除");
    expect(body.querySelectorAll("a").length).toBe(2);
  });
  test("已知语言高亮，未知语言安全转义", () => {
    const known = renderMarkdown('```javascript\nconst x = "<script>";\n```');
    expect(known).toContain("hljs-keyword");
    expect(parse(known).querySelector("script")).toBeNull();
    const unknown = renderMarkdown("```unknown\n<img src=x onerror=alert(1)>\n```");
    expect(parse(unknown).querySelector("code")?.textContent).toContain("<img src=x onerror=alert(1)>");
    expect(parse(unknown).querySelector("img")).toBeNull();
  });
});

describe("扩展原型语法", () => {
  test("脚注、标记、上下标及 frontmatter", () => {
    const body = parse(renderMarkdown("---\ntitle: 隐藏元数据\n---\n\n正文[^1] ==重点== H~2~O x^2^\n\n[^1]: 脚注内容"));
    expect(body.textContent).not.toContain("隐藏元数据");
    expect(body.querySelector("mark")?.textContent).toBe("重点");
    expect(body.querySelector("sub")?.textContent).toBe("2");
    expect(body.querySelectorAll("sup").length).toBe(2);
    expect(body.querySelector(".footnotes")?.textContent).toContain("脚注内容");
    const target = body.querySelector(".footnote-ref a")?.getAttribute("href")?.slice(1);
    expect(target).toBeTruthy();
    expect(body.querySelector(`[id="${target}"]`)).not.toBeNull();
    expect(renderMarkdown("---\n\n正文")).toContain("<hr>");
  });
  test("提示块与 details 容器，标题转义", () => {
    const body = parse(renderMarkdown("> [!NOTE] 提示\n> **内容**\n\n::: details 查看详情 <img src=x onerror=alert(1)>\n正文\n:::"));
    expect(body.querySelector(".callout-note strong")?.textContent).toBe("提示");
    expect(body.querySelector("details summary")?.textContent).toContain("<img");
    expect(body.querySelector("summary img")).toBeNull();
    expect(body.querySelector("details p")?.textContent).toBe("正文");
  });
  test("KaTeX 输出内联和块级 MathML，无外链样式", () => {
    const html = renderMarkdown("公式 $x^2$\n\n$$\n\\frac{1}{2}\n$$");
    const body = parse(html);
    expect(body.querySelectorAll("math").length).toBe(2);
    expect(body.querySelector("math[display=block] mfrac")).not.toBeNull();
    expect(body.querySelector("msup")).not.toBeNull();
    expect(exportHTML("公式", "$x^2$")).not.toContain("<link");
    expect(renderMarkdown("价格 $10 和 $20")).not.toContain("<math");
    expect(parse(renderMarkdown("$\\unknown{<script>}$")).querySelector("script")).toBeNull();
    expect(parse(renderMarkdown("$\\href{javascript:alert(1)}{点击}$")).querySelector("[href]")).toBeNull();
  });
  test.each(["png", "jpeg", "jpg", "webp", "gif"])("仅图片允许 %s base64 data URI", type => {
    const url = `data:image/${type};base64,AAAA`;
    const body = parse(renderMarkdown(`![描述](${url}) [链接](${url})`));
    expect(body.querySelector("img")?.getAttribute("src")).toBe(url);
    expect(body.querySelector("a[href]")).toBeNull();
    expect(isSafeURL(url)).toBe(false);
  });
  test("真实笔记四种 wiki 引用显示别名并安全编码目标", () => {
    const body = parse(renderMarkdown("[[文件|别名]] [[文件#标题|别名]] [[#标题]] [[#^块]]"));
    const links = [...body.querySelectorAll("a.wiki-link")];
    expect(links.map(link => link.textContent)).toEqual(["别名", "别名", "标题", "^块"]);
    expect(links.map(link => link.getAttribute("href"))).toEqual(["./%E6%96%87%E4%BB%B6", "./%E6%96%87%E4%BB%B6#%E6%A0%87%E9%A2%98", "#%E6%A0%87%E9%A2%98", "#%5E%E5%9D%97"]);
  });
  test("同文档标题与段落块链接具有对应锚点", () => {
    const body = parse(renderMarkdown("[[#Callout 块]] [[#^block-sample]]\n\n## Callout 块\n\n段落内容。 ^block-sample"));
    expect(body.querySelector('h2[id="Callout 块"]')).not.toBeNull();
    expect(body.querySelector('p[id="^block-sample"]')?.textContent).toBe("段落内容。");
  });
  test("未解析嵌入明确占位，代码和转义 wiki 保持原文", () => {
    const body = parse(renderMarkdown("![[图片.svg|480]] ![[笔记#标题]] ![[音频.mp3]] ![[文档.pdf#page=1]]\n\n`[[代码]]` \\[[转义]]\n\n```md\n[[代码块]]\n```"));
    expect(body.querySelectorAll(".embed-placeholder")).toHaveLength(4);
    expect(body.querySelectorAll("a.wiki-link")).toHaveLength(0);
    expect(body.textContent).toContain("暂未解析");
    expect(body.querySelector("code")?.textContent).toBe("[[代码]]");
  });
  test("wiki 目标和别名不能注入协议、HTML 或属性", () => {
    const body = parse(renderMarkdown('[[javascript:alert(1)|点击]] [[//evil.example/x|网络]] [[文件|<img src=x onerror=alert(1)>]]'));
    expect(body.querySelectorAll("img,script")).toHaveLength(0);
    for (const link of body.querySelectorAll("a")) expect(link.getAttribute("href")?.startsWith("./")).toBe(true);
    expect(body.textContent).toContain("<img src=x onerror=alert(1)>");
  });
  test("Callout 别名及嵌套类型保留标题与正文", () => {
    const body = parse(renderMarkdown("> [!faq]- 默认折叠\n> 内容\n>\n> > [!tip]\n> > 内层"));
    expect(body.querySelector(".callout-question strong")?.textContent).toBe("默认折叠");
    expect(body.querySelector(".callout-tip strong")?.textContent).toBe("tip");
    expect(body.textContent).toContain("内层");
  });
  test("Mermaid 保留为安全代码块", () => {
    const body = parse(renderMarkdown("```mermaid\ngraph TD; A-->B\n```"));
    expect(body.querySelector("code.language-mermaid")?.textContent).toContain("graph TD");
    expect(body.querySelector("svg")).toBeNull();
  });
});

describe("HTML 安全", () => {
  test.each([
    '<script>alert(1)</script>', '<img src=x onerror="alert(1)">', '<svg onload="alert(1)"><script>alert(1)</script></svg>',
    '[脚本](javascript:alert(1))', '[实体](jav&#x61;script:alert(1))', '[控制](java&#x09;script:alert(1))',
    '![图片](data:image/svg+xml;base64,PHN2Zz4=)', '[文件](file:///etc/passwd)', '[协议](vbscript:alert(1))',
    '[数据](data:text/html;base64,PHNjcmlwdD4=)',
  ])("拒绝可执行载荷 %s", input => {
    const body = parse(renderMarkdown(input));
    expect(body.querySelectorAll("script,svg,iframe,object,embed").length).toBe(0);
    for (const element of body.querySelectorAll("*")) {
      for (const attribute of element.attributes) {
        expect(attribute.name.startsWith("on")).toBe(false);
        if (["href", "src"].includes(attribute.name)) expect(isSafeURL(attribute.value)).toBe(true);
      }
    }
  });
  test.each(["javascript:alert(1)", "JaVaScRiPt:alert(1)", "java\tscript:alert(1)", "\u0000data:text/html,x", "vbscript:x", "file:///x"])("URL 拒绝 %s", url => {
    expect(isSafeURL(url)).toBe(false);
  });
  test.each(["https://example.com", "http://example.com", "mailto:a@example.com", "tel:+123", "/image.png", "../image.png", "#标题"])("URL 接受 %s", url => {
    expect(isSafeURL(url)).toBe(true);
  });
});

test("导出包含标题、正文、内嵌主题和打印样式，转义名称", () => {
  const html = exportHTML('</title><script>alert(1)</script>', "# 正文\n\n**内容**", "dark");
  expect(html.startsWith("<!doctype html>")).toBe(true);
  expect(html).toContain('data-theme="dark"');
  expect(html).toContain("@media print");
  expect(html).toContain("<h1>正文</h1>");
  const window = new JSDOM("").window;
  window.document.write(html);
  expect(window.document.title).toBe('</title><script>alert(1)</script>');
  expect(window.document.querySelector("script")).toBeNull();
  expect(window.document.querySelectorAll("link,script").length).toBe(0);
  expect(exportHTML("名称", "")).toContain('data-theme="light"');
  expect(exportHTML("名称", "", "auto")).toContain('data-theme="auto"');
});
