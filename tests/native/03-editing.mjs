import { page, selectFolder, shell } from "./steps.mjs";

// 表格、代码块、公式、查找替换与格式工具栏。
export default ({ workspace, workspace2, out }) => [
  selectFolder("切换到 full-ws", workspace),
  page(
    "重置设置并打开山中来信",
    async () => {
      window.__xss = 0;
      const s = await resetSettings();
      if (s.fontSize !== 15 || s.readingWidth !== "comfort") throw Error("恢复默认失败 " + JSON.stringify(s));
      await refreshWorkspace("full-ws");
      await openFile("山中来信.md");
      await blur();
      return { settings: s, tables: $$(".lm-table-preview").length, blocks: $$(".lm-block-preview").length, chars: doc().length };
    },
    { shot: "c01-live", focus: true },
  ),
  page(
    "源码切换：进入源码",
    async () => {
      window.__src = { text: doc(), scroll: V.scrollDOM.scrollTop };
      const pos = doc().indexOf("|");
      V.dispatch({ selection: { anchor: pos } });
      $("#mode-toggle").click();
      await sleep(200);
      const r = {
        mode: $("#edit-mode").textContent,
        btn: $("#mode-toggle").textContent,
        tables: $$(".lm-table-preview").length,
        blocks: $$(".lm-block-preview").length,
        widgets: $$(".cm-content .cm-widgetBuffer, .cm-content [contenteditable=false]").length,
        rawPipes: $(".cm-content").textContent.includes("| ---") || $(".cm-content").textContent.includes("|---"),
        hashes: $(".cm-content").textContent.trimStart().startsWith("#"),
        same: doc() === __src.text,
        anchor: V.state.selection.main.anchor === pos,
        highlightSpans: $$(".cm-content span[class]").length,
      };
      if (r.tables || r.blocks || !r.same || !r.hashes) throw Error("源码模式不符 " + JSON.stringify(r));
      return r;
    },
    { shot: "c02-source" },
  ),
  page("源码模式：编辑、撤销栈与格式工具栏", async () => {
    const before = doc();
    V.focus();
    V.dispatch({ changes: { from: 0, insert: "源码插入\n" }, userEvent: "input.type" });
    const from = doc().indexOf("山中");
    V.dispatch({ selection: { anchor: from, head: from + 2 } });
    await sleep(150);
    const toolbarHidden = $("#format-toolbar").hidden;
    $("#mode-toggle").click();
    await sleep(200);
    const liveHas = doc().startsWith("源码插入");
    const undone = T.undo();
    const restored = doc() === before;
    await T.flush();
    await blur();
    return {
      focus: V.hasFocus,
      toolbarHiddenInSource: toolbarHidden,
      liveHas,
      undone,
      跨模式撤销还原: restored,
      tables: $$(".lm-table-preview").length,
      blocks: $$(".lm-block-preview").length,
      mode: $("#edit-mode").textContent,
    };
  }),
  page("源码切换：阅读模式下点击", async () => {
    $("#reading-toggle").click();
    await sleep(300);
    const reading = !$("#reading-view").hidden;
    // 阅读模式下状态栏如实显示；点“源码”先退出阅读，再进入源码编辑。
    const label = $("#edit-mode").textContent;
    $("#mode-toggle").click();
    await sleep(150);
    const r = { reading, label, stillReading: !$("#reading-view").hidden, mode: $("#edit-mode").textContent, editorHidden: $("#editor").hidden };
    if ($("#edit-mode").textContent === "源码编辑") $("#mode-toggle").click();
    if (!$("#reading-view").hidden) $("#reading-toggle").click();
    await sleep(150);
    if (!reading || label !== "阅读模式" || r.stillReading || r.mode !== "源码编辑" || r.editorHidden) throw Error("阅读模式下切源码不符 " + JSON.stringify(r));
    eq($("#edit-mode").textContent, "原位编辑", "恢复原位编辑");
    return r;
  }),
  page(
    "表格：插入默认表格",
    async () => {
      await newFile("表格测试.md");
      V.focus();
      $("#more-actions").click();
      await sleep(40);
      $$(".action-menu button")
        .find(b => b.textContent === "格式与插入…")
        .click();
      await sleep(60);
      const items = $$(".action-menu button").map(b => b.textContent);
      $$(".action-menu button")
        .find(b => b.textContent === "插入表格")
        .click();
      await sleep(150);
      eq(doc(), "| 列 1 | 列 2 |\n| --- | --- |\n| 内容 | 内容 |", "默认表格");
      await blur();
      return { items, preview: $$(".lm-table-preview").length, sel: V.state.selection.main.head };
    },
    { shot: "c03-table-inserted" },
  ),
  page("表格：光标不在表格时的提示", async () => {
    setDoc("前言\n\n| 列 1 | 列 2 |\n| --- | --- |\n| 内容 | 内容 |", 0);
    await menu("#format-menu-toggle", "编辑当前表格…");
    const r = { dialog: !!D(), toast: $("#toast").hidden ? "" : $("#toast").textContent };
    if (r.dialog || !r.toast) throw Error("应提示而非打开 " + JSON.stringify(r));
    setDoc("| 列 1 | 列 2 |\n| --- | --- |\n| 内容 | 内容 |", 0);
    return r;
  }),
  page(
    "表格编辑器：编辑单元格、增行增列、移动、对齐",
    async () => {
      await menu("#format-menu-toggle", "编辑当前表格…");
      if (!D()?.open) throw Error("表格编辑器未打开");
      const dims = () => D().querySelector(".lm-detail-header span").textContent;
      const out = { dims0: dims(), focus0: document.activeElement.getAttribute("aria-label") };
      type(cell(0, 0), "名称");
      type(cell(0, 1), "数量");
      type(cell(1, 0), "苹果");
      type(cell(1, 1), "3");
      out.cellInputSync = D().querySelector(".lm-detail-input-row input").value;
      tb("下方插入").click();
      out.afterRow = [dims(), D().querySelector(".lm-table-status span").textContent];
      type(cell(2, 0), "梨|桃");
      type(cell(2, 1), "5");
      tb("右侧插入").click();
      out.afterCol = [dims(), D().querySelector(".lm-table-status span").textContent];
      type(cell(0, 2), "备注");
      cell(0, 1).focus();
      tb("右对齐").click();
      out.alignPressed = tb("右对齐").getAttribute("aria-pressed");
      out.inputAlign = cell(1, 1).style.textAlign;
      cell(2, 0).focus();
      tb("行上移").click();
      out.afterMoveRow = [cell(1, 0).value, cell(2, 0).value, tb("行上移").disabled];
      cell(0, 2).focus();
      tb("列左移").click();
      out.afterMoveCol = [cell(0, 0).value, cell(0, 1).value, cell(0, 2).value];
      cell(0, 0).focus();
      out.headerGuards = {
        删除行: tb("删除行").disabled,
        上方插入: tb("上方插入").disabled,
        行上移: tb("行上移").disabled,
        行下移: tb("行下移").disabled,
        列左移: tb("列左移").disabled,
      };
      return out;
    },
    { shot: "c04-table-editor" },
  ),
  page(
    "表格编辑器：完成写回 Markdown",
    async () => {
      D().querySelector("form").requestSubmit();
      await sleep(150);
      if (D()) throw Error("对话框未关闭");
      const expected = "| 名称 | 备注 | 数量 |\n| --- | --- | ---: |\n| 梨\\|桃 |  | 5 |\n| 苹果 |  | 3 |";
      eq(doc(), expected, "写回结果");
      const focused = V.hasFocus;
      await blur();
      const cells = $$(".lm-table-preview td").map(td => td.textContent);
      const align = $$(".lm-table-preview th").map(th => th.style.textAlign);
      return { focused, cells, align, dirty: T.document().dirty };
    },
    { shot: "c05-table-result" },
  ),
  page("表格编辑器：整体一次撤销与重做", async () => {
    const after = doc();
    const u = T.undo();
    const afterUndo = doc();
    const r = T.redo();
    eq(afterUndo, "| 列 1 | 列 2 |\n| --- | --- |\n| 内容 | 内容 |", "撤销一步");
    eq(doc(), after, "重做");
    return { u, r };
  }),
  page("表格编辑器：取消与 Escape 不改文档", async () => {
    const before = doc();
    V.dispatch({ selection: { anchor: 3 } });
    await menu("#format-menu-toggle", "编辑当前表格…");
    type(cell(1, 0), "不应保存");
    tb("取消").click();
    await sleep(80);
    const a = doc() === before && !D();
    await menu("#format-menu-toggle", "编辑当前表格…");
    type(cell(1, 0), "不应保存2");
    D().dispatchEvent(new Event("cancel", { cancelable: true }));
    await sleep(80);
    const b = doc() === before && !D();
    if (!a || !b) throw Error("取消后文档变化 " + a + b);
    return { cancel: a, escape: b, focusBack: V.hasFocus };
  }),
  page(
    "表格预览：双击与菜单进入编辑器",
    async () => {
      await blur();
      const p = $(".lm-table-preview");
      p.querySelector("table").dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));
      await sleep(120);
      const byDbl = !!D()?.open;
      tb("取消").click();
      await sleep(80);
      await blur();
      $(".lm-table-preview .lm-table-actions").click();
      await sleep(60);
      const items = $$(".lm-table-preview [role=menuitem]").map(i => i.textContent);
      $$(".lm-table-preview [role=menuitem]")
        .find(i => i.textContent === "编辑表格")
        .click();
      await sleep(120);
      const byMenu = !!D()?.open;
      return { byDbl, byMenu, items };
    },
    { shot: "c06-table-editor-from-preview" },
  ),
  page("表格编辑器：删行删列与最小约束", async () => {
    const out = {};
    cell(1, 0).focus();
    tb("删除行").click();
    tb("删除行").click();
    out.rowsLeft = D().querySelectorAll("tr").length - 1;
    out.deleteRowDisabledNoRows = tb("删除行").disabled;
    cell(0, 0).focus();
    tb("删除列").click();
    tb("删除列").click();
    out.cols = D().querySelectorAll("tr:first-child input").length;
    out.deleteColDisabled = tb("删除列").disabled;
    D().querySelector("form").requestSubmit();
    await sleep(120);
    out.doc = doc();
    await blur();
    out.preview = $$(".lm-table-preview").length;
    return out;
  }),
  page("表格编辑器：文档已变化时拒绝写回", async () => {
    setDoc("| a | b |\n| --- | --- |\n| 1 | 2 |\n\n尾部", 0);
    await menu("#format-menu-toggle", "编辑当前表格…");
    type(cell(1, 0), "X");
    V.dispatch({ changes: { from: V.state.doc.length, insert: " 外部变化" } });
    D().querySelector("form").requestSubmit();
    await sleep(80);
    const err = D()?.querySelector(".lm-detail-error").textContent;
    const open = !!D();
    if (open) tb("取消").click();
    await sleep(60);
    if (!err) throw Error("未提示文档变化");
    return { err, open, unchanged: doc().includes("| 1 | 2 |") };
  }),
  page("表格：引用块内表格保留前缀", async () => {
    setDoc("> | a | b |\n> | --- | --- |\n> | 1 | 2 |", 5);
    await menu("#format-menu-toggle", "编辑当前表格…");
    if (!D()) return { supported: false, toast: $("#toast").textContent };
    cell(1, 0).focus();
    tb("下方插入").click();
    type(cell(2, 0), "3");
    D().querySelector("form").requestSubmit();
    await sleep(100);
    return { supported: true, doc: doc() };
  }),
  page(
    "表格：单元格行内格式、对齐、转义与 HTML 的原位预览",
    async () => {
      setDoc(
        '| 左 | 中 | 右 |\n| :--- | :---: | ---: |\n| **粗** | [链接](https://example.com) | `a\\|b` |\n| <img src=x onerror="window.__xss=1"> | ==高亮== | ![图](./图.png) |\n\n结尾',
        0,
      );
      await blur();
      const p = $(".lm-table-preview");
      const tds = [...p.querySelectorAll("td")];
      const r = {
        cells: tds.map(t => t.textContent),
        html: tds.map(t => t.innerHTML).slice(0, 4),
        align: [...p.querySelectorAll("th")].map(t => t.style.textAlign),
        imgs: p.querySelectorAll("img").length,
        xss: window.__xss,
      };
      // 单元格内的加粗、链接、行内代码和高亮都渲染出来；图片只显示替代文本，原始 HTML 仍是文字。
      eq(r.html[0], "<strong>粗</strong>", "加粗");
      eq(tds[1].querySelector("a")?.getAttribute("href"), "https://example.com", "链接");
      eq(tds[2].querySelector("code")?.textContent, "a|b", "行内代码与转义管道符");
      eq(tds[4].querySelector("mark")?.textContent, "高亮", "高亮");
      eq(tds[5].querySelector(".lm-image-fallback")?.textContent, "图", "图片替代文本");
      eq(r.align.join(), "left,center,right", "对齐");
      if (r.imgs || r.xss || !r.cells[3].startsWith("<img")) throw Error("原始 HTML 未按文字显示 " + JSON.stringify(r));
      $("#reading-toggle").click();
      await sleep(400);
      const rv = $("#reading-view");
      r.reading = {
        strong: rv.querySelectorAll("td strong").length,
        links: rv.querySelectorAll("td a").length,
        mark: rv.querySelectorAll("td mark").length,
        imgs: rv.querySelectorAll("td img").length,
        align: [...rv.querySelectorAll("th")].map(t => t.style.textAlign || t.getAttribute("align")),
        xss: window.__xss,
      };
      return r;
    },
    { shot: "c07-table-reading" },
  ),
  page(
    "表格：原位预览截图（行内格式）",
    async () => {
      $("#reading-toggle").click();
      await sleep(200);
      await blur();
      return { reading: !$("#reading-view").hidden };
    },
    { shot: "c08-table-inline-live" },
  ),
  page(
    "表格：光标进入显示源码、移出恢复预览",
    async () => {
      V.focus();
      await sleep(100);
      V.dispatch({ selection: { anchor: 3 } });
      await sleep(200);
      const inside = $$(".lm-table-preview").length;
      V.dispatch({ selection: { anchor: V.state.doc.length } });
      await sleep(200);
      const outside = $$(".lm-table-preview").length;
      return { focus: V.hasFocus, inside, outside };
    },
    { shot: "c09-table-source-on-cursor", focus: true },
  ),
  page("表格：保存到磁盘", async () => {
    $("#save-file").click();
    await until(() => !T.document().dirty, "保存");
    return { status: $("#save-status").textContent };
  }),
  shell("磁盘内容：表格测试.md", `cat "${workspace}/表格测试.md"`),
  page(
    "代码块：空行插入并输入",
    async () => {
      await newFile("代码测试.md");
      V.focus();
      await menu("#format-menu-toggle", "代码块");
      const a = { doc: doc(), head: V.state.selection.main.head };
      eq(a.doc, F + "\n\n" + F, "空代码块");
      V.dispatch({ changes: { from: a.head, insert: "const a = 1;\nconsole.log(a);" }, userEvent: "input.type" });
      V.dispatch({ changes: { from: 3, insert: "js" }, userEvent: "input.type" });
      const d = doc();
      eq(d, F + "js\nconst a = 1;\nconsole.log(a);\n" + F, "代码内容");
      const whileEditing = $$(".lm-code-preview").length;
      V.dispatch({ changes: { from: V.state.doc.length, insert: "\n\n结尾段落" } });
      await blur();
      const p = $(".lm-code-preview");
      if (!p) throw Error("未渲染代码预览");
      return {
        cursorInside: a.head,
        focus: V.hasFocus,
        编辑时预览数: whileEditing,
        lang: p.querySelector(".lm-block-language")?.textContent,
        hl: p.querySelectorAll("code span[class^=hljs]").length,
        text: p.querySelector("code").textContent,
      };
    },
    { shot: "c10-code-preview", focus: true },
  ),
  page(
    "代码块：编辑按钮显示源码并可修改",
    async () => {
      const p = $(".lm-code-preview");
      p.querySelector("[aria-label=编辑代码源码]").click();
      await sleep(200);
      const r = {
        previewGone: $$(".lm-code-preview").length === 0,
        focus: V.hasFocus,
        head: V.state.selection.main.head,
        lineAtHead: V.state.doc.lineAt(V.state.selection.main.head).text,
      };
      if (!r.previewGone) throw Error("点击编辑后仍是预览");
      V.dispatch({ changes: { from: V.state.selection.main.head, insert: "// 新增注释\n" }, userEvent: "input.type" });
      await sleep(100);
      r.stillSource = $$(".lm-code-preview").length === 0;
      return r;
    },
    { shot: "c11-code-editing" },
  ),
  page("代码块：光标移出后重新渲染", async () => {
    V.dispatch({ selection: { anchor: V.state.doc.length } });
    await sleep(200);
    const p = $(".lm-code-preview");
    if (!p) throw Error("移出后未恢复预览");
    const text = p.querySelector("code").textContent;
    if (!text.startsWith("// 新增注释")) throw Error("预览未更新 " + text);
    p.querySelector(".lm-block-content").dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));
    await sleep(150);
    const dbl = $$(".lm-code-preview").length === 0;
    V.dispatch({ selection: { anchor: V.state.doc.length } });
    await sleep(150);
    return { text, 双击进入源码: dbl, hl: $(".lm-code-preview").querySelectorAll("span[class^=hljs]").length };
  }),
  shell("备份剪贴板", `pbpaste > ${out}/clip.bak; echo saved`),
  page(
    "代码块：复制按钮",
    async () => {
      const p = $(".lm-code-preview");
      const before = doc();
      p.querySelector("[aria-label=复制代码]").click();
      await until(() => p.querySelector(".lm-block-status").textContent, "复制状态");
      return { status: p.querySelector(".lm-block-status").textContent, 仍是预览: $$(".lm-code-preview").length === 1, unchanged: doc() === before };
    },
    { shot: "c12-code-copied" },
  ),
  shell("剪贴板内容并还原", `pbpaste; echo; echo ---; pbcopy < ${out}/clip.bak; echo restored`),
  page("代码块：选中多行包裹与再次取消", async () => {
    setDoc("第一行\n第二行\n\n段落", 0);
    V.dispatch({ selection: { anchor: 0, head: 7 } });
    V.focus();
    await menu("#format-menu-toggle", "代码块");
    const wrapped = doc();
    eq(wrapped, F + "\n第一行\n第二行\n" + F + "\n\n段落", "包裹");
    V.dispatch({ selection: { anchor: 0, head: wrapped.indexOf("\n\n段落") } });
    await menu("#format-menu-toggle", "代码块");
    eq(doc(), "第一行\n第二行\n\n段落", "取消包裹");
    setDoc("含 " + F + " 的行", 0);
    V.dispatch({ selection: { anchor: 0, head: V.state.doc.length } });
    await menu("#format-menu-toggle", "代码块");
    return { wrapped, innerFence: doc() };
  }),
  page(
    "代码块：未知语言、HTML 转义、未闭合、波浪线围栏",
    async () => {
      setDoc(
        F +
          "foolang\n<b>x</b>\n" +
          F +
          "\n\n" +
          F +
          'html\n<script>window.__xss=2</script><img src=x onerror="window.__xss=3">\n' +
          F +
          "\n\n~~~python\nprint(1)\n~~~\n\n" +
          F +
          "\n无语言\n" +
          F +
          "\n\n段落\n\n" +
          F +
          "js\n未闭合",
        0,
      );
      await blur();
      const ps = $$(".lm-code-preview");
      return {
        count: ps.length,
        langs: ps.map(p => p.querySelector(".lm-block-language")?.textContent ?? null),
        hl: ps.map(p => p.querySelectorAll("span[class^=hljs]").length),
        text0: ps[0]?.querySelector("code").textContent,
        bold: ps[0]?.querySelectorAll("b").length,
        scripts: $$(".lm-code-preview script, .lm-code-preview img").length,
        xss: window.__xss,
        未闭合保持源码: $(".cm-content").textContent.includes("未闭合"),
      };
    },
    { shot: "c13-code-variants" },
  ),
  page(
    "代码块：公式与 Mermaid",
    async () => {
      setDoc("行内 $a^2+b^2$ 与价格 $5 和 $10。\n\n$$\nE = mc^2\n$$\n\n$$\n\\frac{1\n$$\n\n" + F + "mermaid\ngraph TD\n  A-->B\n" + F + "\n\n尾", 0);
      await blur();
      const r = {
        inlineMath: $$(".lm-inline-math").length,
        mathBlocks: $$(".lm-math-preview").length,
        mathml: $$(".lm-math-preview math").length,
        badStatus: $$(".lm-math-preview .lm-block-status").map(s => s.textContent),
        codeBlocks: $$(".lm-code-preview").length,
      };
      // Mermaid 代码块在原位预览里直接渲染成图形。
      await until(() => $("#editor .lm-code-preview figure.mermaid-diagram svg"), "原位 Mermaid 图形");
      const figure = $("#editor figure.mermaid-diagram svg").getBoundingClientRect();
      r.mermaidLive = { width: Math.round(figure.width), height: Math.round(figure.height) };
      // 两个节点的小图按自身尺寸显示，不被拉伸到整行。
      if (figure.width < 40 || figure.height < 60 || figure.width > 300) throw Error("原位 Mermaid 图形尺寸异常 " + JSON.stringify(r.mermaidLive));
      if (r.inlineMath !== 1 || r.mathBlocks !== 2 || r.mathml !== 1 || !r.badStatus[1]) throw Error("公式渲染不符 " + JSON.stringify(r));
      $("#reading-toggle").click();
      await until(() => $("#reading-view figure.mermaid-diagram svg") || $("#reading-view .mermaid-error"), "mermaid");
      r.reading = { mermaidSvg: !!$("#reading-view figure.mermaid-diagram svg"), katex: $$("#reading-view .katex, #reading-view math").length };
      r.reading.width = Math.round($("#reading-view figure.mermaid-diagram svg").getBoundingClientRect().width);
      if (!r.reading.mermaidSvg || r.reading.width > 300) throw Error("阅读模式 Mermaid 不符 " + JSON.stringify(r.reading));
      return r;
    },
    { shot: "c14-math-mermaid-reading" },
  ),
  page(
    "原位下的公式与 Mermaid 截图",
    async () => {
      $("#reading-toggle").click();
      await sleep(200);
      await blur();
      await until(() => $("#editor .lm-code-preview figure.mermaid-diagram svg"), "原位 Mermaid 图形");
      return true;
    },
    { shot: "c15-math-mermaid-live" },
  ),
  page(
    "代码编辑：Tab 缩进、回车续列表",
    async () => {
      setDoc(F + "js\nfoo();\n" + F + "\n\n- 第一项", 0);
      V.focus();
      await sleep(100);
      const p = doc().indexOf("foo");
      V.dispatch({ selection: { anchor: p } });
      key("Tab", { keyCode: 9, code: "Tab" });
      await sleep(60);
      const afterTab = doc().split("\n")[1];
      key("Tab", { keyCode: 9, code: "Tab", shiftKey: true });
      await sleep(60);
      const afterShiftTab = doc().split("\n")[1];
      V.dispatch({ selection: { anchor: V.state.doc.length } });
      key("Enter", { keyCode: 13, code: "Enter" });
      await sleep(60);
      const afterEnter = doc().split("\n").slice(-2);
      return { focus: V.hasFocus, afterTab, afterShiftTab, afterEnter };
    },
    { focus: true },
  ),
  page(
    "源码模式下的代码与表格高亮",
    async () => {
      setDoc("# 标题\n\n" + F + "js\nconst a = 1; // 注释\n" + F + "\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\n**粗** *斜* [链](https://x.y)", 0);
      $("#mode-toggle").click();
      await sleep(250);
      const classes = [...new Set($$(".cm-content span[class]").flatMap(s => [...s.classList]))];
      const r = {
        mode: $("#edit-mode").textContent,
        classes: classes.slice(0, 30),
        colored: new Set($$(".cm-content span[class]").map(s => getComputedStyle(s).color)).size,
      };
      // 源码模式下围栏代码整块使用等宽字体，正文不变。
      const code = $$(".cm-line.md-source-code");
      r.codeLines = code.map(line => line.textContent);
      r.fonts = { code: getComputedStyle(code[1]).fontFamily, body: getComputedStyle($$(".cm-line").at(-1)).fontFamily };
      eq(r.codeLines.join("\n"), F + "js\nconst a = 1; // 注释\n" + F, "等宽代码行");
      if (r.fonts.code === r.fonts.body || !/mono/i.test(r.fonts.code)) throw Error("源码模式代码不是等宽 " + JSON.stringify(r.fonts));
      if (r.mode !== "源码编辑" || r.colored < 4) throw Error("源码高亮不符 " + JSON.stringify(r));
      return r;
    },
    { shot: "c16-source-highlight" },
  ),
  page("回到原位", async () => {
    $("#mode-toggle").click();
    await sleep(200);
    await blur();
    return { mode: $("#edit-mode").textContent, blocks: $$(".lm-block-preview").length, tables: $$(".lm-table-preview").length };
  }),
  page(
    "查找与替换",
    async () => {
      setDoc("苹果 apple Apple\n\n苹果派与苹果汁\n\n| 苹果 | 数量 |\n| --- | --- |\n| 青苹果 | 2 |", 0);
      V.focus();
      $("#more-actions").click();
      await sleep(40);
      $$(".action-menu button")
        .find(b => b.textContent === "查找与替换")
        .click();
      await sleep(150);
      const panel = $(".cm-search");
      if (!panel) throw Error("查找面板未出现");
      const labels = [...panel.querySelectorAll("button,label")].map(e => e.textContent.trim());
      const s = panel.querySelector("input[name=search]"),
        rp = panel.querySelector("input[name=replace]");
      const set = (el, v) => {
        el.value = v;
        el.dispatchEvent(new Event("change", { bubbles: true }));
        el.dispatchEvent(new KeyboardEvent("keyup", { bubbles: true }));
      };
      set(s, "苹果");
      await sleep(100);
      const matches = $$(".cm-searchMatch").length;
      panel.querySelector("button[name=next]").click();
      await sleep(80);
      const sel1 = V.state.sliceDoc(V.state.selection.main.from, V.state.selection.main.to);
      const tablesWhileSearch = $$(".lm-table-preview").length;
      set(rp, "梨");
      panel.querySelector("button[name=replace]").click();
      await sleep(80);
      const afterOne = doc().split("\n")[0];
      panel.querySelector("button[name=replaceAll]").click();
      await sleep(100);
      const all = doc();
      set(s, "APPLE");
      await sleep(80);
      const ci = $$(".cm-searchMatch").length;
      // 查找命中表格里的文字时，表格退回源文显示，匹配全部高亮。
      if (matches !== 5 || tablesWhileSearch !== 0 || sel1 !== "苹果") throw Error("查找高亮不符 " + JSON.stringify({ matches, tablesWhileSearch, sel1 }));
      eq(all, "梨 apple Apple\n\n梨派与梨汁\n\n| 梨 | 数量 |\n| --- | --- |\n| 青梨 | 2 |", "全部替换");
      eq(ci, 2, "默认不区分大小写");
      return { labels, placeholder: [s.placeholder, rp.placeholder], matches, sel1, tablesWhileSearch, afterOne, all, 默认不区分大小写匹配数: ci };
    },
    { shot: "c17-find-replace", focus: true },
  ),
  page("查找：撤销全部替换、Escape 关闭", async () => {
    const u = T.undo();
    const afterUndo = doc();
    const panel = $(".cm-search");
    panel.querySelector("input[name=search]").dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", keyCode: 27, bubbles: true, cancelable: true }));
    await sleep(100);
    const closedByEsc = !$(".cm-search");
    if (!closedByEsc) panel.querySelector("button[name=close]").click();
    await sleep(60);
    await blur();
    const tables = $$(".lm-table-preview").length;
    if (!closedByEsc || tables !== 1 || !afterUndo.includes("苹果派与苹果汁"))
      throw Error("关闭查找后的状态不符 " + JSON.stringify({ closedByEsc, tables, afterUndo }));
    return { u, closedByEsc, tables };
  }),
  page("查找：阅读模式下打开会退回编辑", async () => {
    $("#reading-toggle").click();
    await sleep(200);
    $("#find-toggle").click();
    await sleep(150);
    const r = { reading: !$("#reading-view").hidden, panel: !!$(".cm-search") };
    $(".cm-search button[name=close]")?.click();
    if (r.reading || !r.panel) throw Error("阅读模式下查找未退回编辑 " + JSON.stringify(r));
    return r;
  }),
  page(
    "格式工具栏：全部按钮",
    async () => {
      const run = async (fmt, text, sel) => {
        setDoc(text, 0);
        V.focus();
        V.dispatch({ selection: { anchor: sel[0], head: sel[1] } });
        await sleep(120);
        const tbv = !$("#format-toolbar").hidden;
        $('[data-format="' + fmt + '"]').click();
        await sleep(60);
        return [doc(), tbv];
      };
      const out = {};
      for (const f of ["bold", "italic", "strike", "code", "h1", "h2", "h3", "bullet", "ordered", "task", "quote", "hr"])
        out[f] = await run(f, "文字内容", [0, 2]);
      out.boldToggleOff = (await run("bold", "**文字**内容", [2, 4]))[0];
      out.multiLineList = (await run("ordered", "甲\n乙\n丙", [0, 5]))[0];
      out.tableBtn = (await run("table", "段落", [0, 2]))[0];
      out.codeblockBtn = (await run("codeblock", "段落", [0, 2]))[0];
      return { focus: V.hasFocus, out };
    },
    { focus: true },
  ),
  page(
    "链接对话框",
    async () => {
      setDoc("访问官网", 0);
      V.focus();
      V.dispatch({ selection: { anchor: 2, head: 4 } });
      await sleep(120);
      $('[data-format="link"]').click();
      await sleep(150);
      const d = $("dialog[open]");
      if (!d) throw Error("链接对话框未打开");
      const inputs = [...d.querySelectorAll("input")].map(i => [i.getAttribute("aria-label") || i.name || i.placeholder, i.value]);
      return { cls: d.className, inputs, buttons: [...d.querySelectorAll("button")].map(b => b.textContent) };
    },
    { shot: "c18-link-dialog" },
  ),
  page("链接对话框：提交", async () => {
    const d = $("dialog[open]");
    const inputs = [...d.querySelectorAll("input")];
    const url = inputs.find(i => /url|地址|链接/i.test((i.getAttribute("aria-label") || "") + i.name + i.placeholder + i.type)) || inputs[1] || inputs[0];
    url.value = "https://example.com/a b";
    url.dispatchEvent(new Event("input", { bubbles: true }));
    d.querySelector("form")?.requestSubmit();
    await sleep(150);
    return { open: !!$("dialog[open]"), doc: doc(), err: $("dialog[open]")?.textContent.slice(0, 200) };
  }),
  page("清理并收尾", async () => {
    for (const d of $$("dialog[open]")) d.close();
    setDoc("# 收尾\n", 0);
    $("#save-file").click();
    await sleep(500);
    if (window.__xss || toast()) throw Error("收尾状态不符 " + JSON.stringify({ xss: window.__xss, toast: toast() }));
    return { xss: window.__xss };
  }),
];
