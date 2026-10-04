import { page, selectFolder, shell } from "./steps.mjs";

// 源码切换、阅读模式、链接和图片对话框、欢迎页与大文档。
export default ({ workspace, workspace2, out }) => [
  selectFolder("切换到 full-ws", workspace),
  page(
    "刷新并打开山中来信（浅色）",
    async () => {
      await setTheme("light");
      await refreshWorkspace("full-ws");
      await openFile("山中来信.md");
      await blur();
      return {
        tables: $$(".lm-table-preview").length,
        blocks: $$(".lm-block-preview").length,
        chars: doc().length,
        lines: V.state.doc.lines,
        outline: $$("#outline button").length,
      };
    },
    { shot: "d01-letter-live", focus: true },
  ),
  page(
    "源码切换：进入源码（长文档）",
    async () => {
      V.scrollDOM.scrollTop = 600;
      await sleep(150);
      window.__src = { text: doc(), top: V.scrollDOM.scrollTop, topPos: V.lineBlockAtHeight(V.scrollDOM.scrollTop).from };
      $("#mode-toggle").click();
      await sleep(300);
      const c = $(".cm-content").textContent;
      const r = {
        mode: $("#edit-mode").textContent,
        btn: $("#mode-toggle").textContent,
        tables: $$(".lm-table-preview").length,
        blocks: $$(".lm-block-preview").length,
        rawTable: /\|\s*-{3}/.test(doc()),
        same: doc() === __src.text,
        scrollBefore: __src.top,
        scrollAfter: V.scrollDOM.scrollTop,
        topLineBefore: V.state.doc.lineAt(__src.topPos).number,
        topLineAfter: V.state.doc.lineAt(V.lineBlockAtHeight(V.scrollDOM.scrollTop).from).number,
        dirty: T.document().dirty,
      };
      if (r.tables || r.blocks || !r.same || r.mode !== "源码编辑") throw Error("源码模式不符 " + JSON.stringify(r));
      return r;
    },
    { shot: "d02-letter-source" },
  ),
  page(
    "源码模式：编辑、工具栏隐藏、跨模式撤销",
    async () => {
      const before = doc();
      V.focus();
      await sleep(100);
      V.dispatch({ changes: { from: 0, insert: "源码插入\n" }, userEvent: "input.type" });
      const from = doc().indexOf("山");
      V.dispatch({ selection: { anchor: from, head: from + 2 } });
      await sleep(200);
      const toolbarHidden = $("#format-toolbar").hidden;
      $("#mode-toggle").click();
      await sleep(300);
      const liveHas = doc().startsWith("源码插入");
      const sel = V.state.selection.main;
      const selKept = sel.from === from && sel.to === from + 2;
      const undone = T.undo();
      const restored = doc() === before;
      await T.flush();
      await blur();
      if (!toolbarHidden || !restored) throw Error("源码模式行为不符");
      return {
        focus: V.hasFocus,
        toolbarHiddenInSource: toolbarHidden,
        liveHas,
        选区保留: selKept,
        undone,
        跨模式撤销还原: restored,
        tables: $$(".lm-table-preview").length,
        blocks: $$(".lm-block-preview").length,
        mode: $("#edit-mode").textContent,
        dirty: T.document().dirty,
      };
    },
    { focus: true },
  ),
  page("源码切换：连续快速切换 20 次", async () => {
    const before = doc();
    const t = performance.now();
    for (let i = 0; i < 20; i++) $("#mode-toggle").click();
    await sleep(300);
    const r = {
      ms: Math.round(performance.now() - t - 300),
      mode: $("#edit-mode").textContent,
      same: doc() === before,
      setting: JSON.parse(localStorage.getItem("leafmark-settings")).liveRendering,
    };
    await blur();
    r.tables = $$(".lm-table-preview").length;
    return r;
  }),
  page(
    "显示当前语法：光标所在行显示标记",
    async () => {
      const text = doc();
      const pos = text.indexOf("**");
      const probe = async () => {
        V.focus();
        await sleep(100);
        V.dispatch({ selection: { anchor: pos + 3 }, scrollIntoView: true });
        await sleep(250);
        const line = V.domAtPos(pos + 3).node.parentElement.closest(".cm-line");
        return { focus: V.hasFocus, hasMarks: line.textContent.includes("**"), sample: line.textContent.slice(0, 40) };
      };
      const setSyntax = async on => {
        $("#settings-toggle").click();
        await sleep(80);
        const row = $$(".settings-row").find(r => r.querySelector(".settings-label")?.textContent === "显示当前语法");
        const input = row.querySelector("input");
        if (input.checked !== on) input.click();
        await sleep(60);
        $(".settings-close").click();
        await sleep(100);
      };
      await setSyntax(true);
      const on = await probe();
      await setSyntax(false);
      const off = await probe();
      const tablePos = text.indexOf("|");
      V.dispatch({ selection: { anchor: tablePos + 2 } });
      await sleep(200);
      const tableWhenOff = $$(".lm-table-preview").length;
      await setSyntax(true);
      V.focus();
      V.dispatch({ selection: { anchor: tablePos + 2 } });
      await sleep(200);
      const tableWhenOn = $$(".lm-table-preview").length;
      if (on.focus && (!on.hasMarks || off.hasMarks)) throw Error("显示当前语法开关无效 " + JSON.stringify({ on, off }));
      return { on, off, 关闭时光标在表格内仍为预览数: tableWhenOff, 开启时预览数: tableWhenOn };
    },
    { shot: "d03-active-syntax", focus: true },
  ),
  page("阅读模式：状态栏与编辑入口", async () => {
    $("#reading-toggle").click();
    await sleep(300);
    const r = {
      editMode: $("#edit-mode").textContent,
      modeToggle: $("#mode-toggle").textContent,
      modeToggleDisabled: $("#mode-toggle").disabled,
      readingBtn: $("#reading-toggle").textContent,
      anyVisibleExit: $$("button")
        .filter(b => b.getBoundingClientRect().width > 0 && /编辑|阅读/.test(b.textContent + b.title))
        .map(b => b.id || b.textContent),
    };
    $("#more-actions").click();
    await sleep(50);
    r.menu = $$(".action-menu button").map(b => b.textContent);
    document.body.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    await sleep(50);
    $("#reading-toggle").click();
    await sleep(150);
    // 阅读模式下状态栏如实显示，回到编辑后恢复。
    if (r.editMode !== "阅读模式" || r.readingBtn !== "编辑" || $("#edit-mode").textContent !== "原位编辑")
      throw Error("阅读模式状态不符 " + JSON.stringify(r));
    return r;
  }),
  page(
    "链接对话框：提交与编码",
    async () => {
      setDoc("访问官网", 0);
      V.focus();
      await sleep(100);
      V.dispatch({ selection: { anchor: 2, head: 4 } });
      await sleep(150);
      $('[data-format="link"]').click();
      await sleep(150);
      const d = $("dialog[open]");
      const url = d.querySelector('input[placeholder="https://example.com"]');
      const form = d.querySelector("form");
      form.requestSubmit();
      await sleep(80);
      const emptyBlocked = !!$("dialog[open]");
      const emptyErr = d.querySelector(".lm-detail-error")?.textContent;
      url.value = "javascript:alert(1)";
      url.dispatchEvent(new Event("input", { bubbles: true }));
      form.requestSubmit();
      await sleep(80);
      const jsErr = $("dialog[open]") ? d.querySelector(".lm-detail-error")?.textContent : "（已接受）";
      const afterJs = doc();
      if ($("dialog[open]")) {
        url.value = "https://example.com/a b";
        url.dispatchEvent(new Event("input", { bubbles: true }));
        form.requestSubmit();
        await sleep(150);
      }
      return { emptyBlocked, emptyErr, jsErr, afterJs, open: !!$("dialog[open]"), doc: doc() };
    },
    { focus: true },
  ),
  page("链接：光标在已有链接上再次编辑", async () => {
    for (const d of $$("dialog[open]")) d.close();
    setDoc('见 [官网](<https://example.com> "标题") 。', 5);
    V.focus();
    await sleep(100);
    $("#format-menu-toggle").click();
    await sleep(40);
    $$(".action-menu button")
      .find(b => b.textContent === "链接…")
      .click();
    await sleep(150);
    const d = $("dialog[open]");
    const r = { inputs: [...d.querySelectorAll("input")].map(i => i.value) };
    d.querySelector("input").value = "https://leafmark.dev";
    d.querySelector("form").requestSubmit();
    await sleep(150);
    r.doc = doc();
    return r;
  }),
  page(
    "图片对话框",
    async () => {
      for (const d of $$("dialog[open]")) d.close();
      setDoc("", 0);
      V.focus();
      await menu("#format-menu-toggle", "图片…");
      const d = $("dialog[open]");
      if (!d) throw Error("图片对话框未打开");
      const r = { cls: d.className, text: d.textContent.slice(0, 160), buttons: [...d.querySelectorAll("button")].map(b => b.textContent) };
      const url = d.querySelector("input");
      url.value = "./图.png";
      url.dispatchEvent(new Event("input", { bubbles: true }));
      d.querySelector("form").requestSubmit();
      await sleep(300);
      r.doc = doc();
      r.open = !!$("dialog[open]");
      await blur();
      await sleep(400);
      r.img = $$("#editor img").map(i => i.src.slice(0, 30));
      return r;
    },
    { shot: "d07-image-inserted" },
  ),
  page(
    "关闭全部标签后的欢迎页",
    async () => {
      for (let i = 0; i < 10 && $$(".file-tab").length; i++) {
        $(".file-tab .close-tab").click();
        await sleep(200);
        if ($("#unsaved-dialog").open) {
          $("#unsaved-dialog button[value=discard]").click();
          await sleep(250);
        }
      }
      const r = {
        tabs: $$(".file-tab").length,
        welcome: !$("#welcome-view").hidden,
        editorHidden: $("#editor").hidden,
        saveDisabled: $("#save-file").disabled,
        title: document.title,
        words: $("#word-count").textContent,
        status: $("#save-status").textContent,
      };
      $("#mode-toggle").click();
      $("#reading-toggle").click();
      await sleep(100);
      r.afterToggles = { welcome: !$("#welcome-view").hidden, reading: !$("#reading-view").hidden, mode: $("#edit-mode").textContent };
      if ($("#edit-mode").textContent === "源码编辑") $("#mode-toggle").click();
      if (r.tabs || !r.welcome || !r.saveDisabled || !r.afterToggles.welcome || r.afterToggles.reading) throw Error("欢迎页状态不符 " + JSON.stringify(r));
      return r;
    },
    { shot: "d08-welcome" },
  ),
  page(
    "欢迎页新建文档",
    async () => {
      $("#welcome-new").click();
      await until(() => T.document().id && $$(".file-tab").length === 1, "新建");
      await sleep(200);
      // 新建后光标直接落在正文里，可以马上输入。
      const r = { name: T.document().name, welcome: !$("#welcome-view").hidden, focus: document.activeElement === V.contentDOM };
      if (r.welcome || !r.focus) throw Error("欢迎页新建后的状态不符 " + JSON.stringify(r));
      return r;
    },
    { focus: true },
  ),
  page("大文档性能：约 30 万字符", async () => {
    const para = "这是一段用于性能测试的文字，包含 **加粗**、*斜体*、`code` 与 [链接](https://example.com)。\n\n";
    const tbl = "| a | b |\n| --- | --- |\n| 1 | 2 |\n\n";
    const code = F + "js\nconst x = 1;\n" + F + "\n\n";
    let s = "";
    for (let i = 0; i < 1500; i++) {
      s += "## 小节 " + i + "\n\n" + para + para + (i % 10 === 0 ? tbl + code : "");
    }
    const t0 = performance.now();
    setDoc(s, 0);
    await sleep(0);
    const tSet = performance.now() - t0;
    const t1 = performance.now();
    V.dispatch({ changes: { from: 0, insert: "x" }, userEvent: "input.type" });
    const tType = performance.now() - t1;
    const t2 = performance.now();
    $("#mode-toggle").click();
    const tToSource = performance.now() - t2;
    const t3 = performance.now();
    $("#mode-toggle").click();
    const tToLive = performance.now() - t3;
    const t4 = performance.now();
    V.dispatch({ selection: { anchor: V.state.doc.length }, scrollIntoView: true });
    await sleep(50);
    const tEnd = performance.now() - t4 - 50;
    const t5 = performance.now();
    $("#reading-toggle").click();
    await sleep(0);
    const tRead = performance.now() - t5;
    $("#reading-toggle").click();
    return {
      chars: s.length,
      lines: V.state.doc.lines,
      outline: $$("#outline button").length,
      ms: {
        setDoc: Math.round(tSet),
        单次输入: Math.round(tType),
        切源码: Math.round(tToSource),
        切原位: Math.round(tToLive),
        跳到末尾: Math.round(tEnd),
        阅读渲染: Math.round(tRead),
      },
      words: $("#word-count").textContent,
    };
  }),
  page("大文档收尾", async () => {
    setDoc("", 0);
    await T.flush();
    return true;
  }),
];
