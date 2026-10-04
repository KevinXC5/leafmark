import { page, selectFolder, shell } from "./steps.mjs";

// 代码与行内元素的配色取值、未保存关闭的三条路径。
export default ({ workspace, workspace2, out }) => [
  selectFolder("切换到 full-ws", workspace),
  page(
    "刷新并打开日记",
    async () => {
      await setTheme("light");
      // 未保存关闭的三条路径以草稿留在内存为前提，先关掉默认开启的自动保存。
      await openS();
      await tab("编辑器");
      const autoSave = rowOf("自动保存").querySelector("input");
      if (autoSave.checked) autoSave.click();
      await closeS();
      if (stored().autoSave !== false) throw Error("自动保存未关闭 " + JSON.stringify(stored()));
      await refreshWorkspace("full-ws");
      await openFile("日记.md", "笔记");
      return { name: T.document().name, doc: doc() };
    },
    { focus: true },
  ),
  page(
    "代码高亮配色：浅色",
    async () => {
      window.__orig = doc();
      setDoc(F + 'js\nconst a = "s"; // c\nfunction f(){ return 1 }\n' + F + "\n\n尾", 0);
      V.dispatch({ selection: { anchor: V.state.doc.length } });
      await blur();
      const p = $(".lm-code-preview");
      const col = s => {
        const e = p.querySelector(s);
        return e ? getComputedStyle(e).color : null;
      };
      const r = {
        theme: document.documentElement.dataset.theme,
        classes: [...new Set([...p.querySelectorAll("span[class]")].map(e => e.className))],
        base: getComputedStyle(p.querySelector("code")).color,
        keyword: col(".hljs-keyword"),
        string: col(".hljs-string"),
        number: col(".hljs-number"),
        comment: col(".hljs-comment"),
        title: col(".hljs-title"),
        bg: getComputedStyle(p).backgroundColor,
      };
      // 语法成分统一用强调色，注释用弱化色，都与代码正文色区分。
      const accents = new Set([r.keyword, r.string, r.number, r.title]);
      if (accents.has(null) || accents.size !== 1 || !r.comment || new Set([r.keyword, r.comment, r.base]).size !== 3) throw Error("代码高亮配色不符 " + JSON.stringify(r));
      return r;
    },
    { shot: "e01-code-light" },
  ),
  page(
    "代码高亮配色：深色",
    async () => {
      await setTheme("dark");
      await sleep(250);
      const p = $(".lm-code-preview");
      const col = s => {
        const e = p.querySelector(s);
        return e ? getComputedStyle(e).color : null;
      };
      const r = {
        theme: document.documentElement.dataset.theme,
        classes: [...new Set([...p.querySelectorAll("span[class]")].map(e => e.className))],
        base: getComputedStyle(p.querySelector("code")).color,
        keyword: col(".hljs-keyword"),
        string: col(".hljs-string"),
        number: col(".hljs-number"),
        comment: col(".hljs-comment"),
        title: col(".hljs-title"),
        bg: getComputedStyle(p).backgroundColor,
      };
      // 语法成分统一用强调色，注释用弱化色，都与代码正文色区分。
      const accents = new Set([r.keyword, r.string, r.number, r.title]);
      if (accents.has(null) || accents.size !== 1 || !r.comment || new Set([r.keyword, r.comment, r.base]).size !== 3) throw Error("代码高亮配色不符 " + JSON.stringify(r));
      $("#reading-toggle").click();
      await sleep(400);
      const rv = $("#reading-view pre code");
      const c = s => (rv.querySelector(s) ? getComputedStyle(rv.querySelector(s)).color : null);
      r.reading = {
        base: getComputedStyle(rv).color,
        keyword: c(".hljs-keyword"),
        string: c(".hljs-string"),
        comment: c(".hljs-comment"),
        number: c(".hljs-number"),
      };
      $("#reading-toggle").click();
      await sleep(200);
      await blur();
      // 阅读模式与原位预览使用同一套配色。
      for (const name of ["keyword", "string", "comment", "number"]) eq(r.reading[name], r[name], "阅读模式 " + name);
      return r;
    },
    { shot: "e02-code-dark" },
  ),
  page(
    "行内配色：链接、高亮与已完成任务",
    async () => {
      const colors = async theme => {
        await setTheme(theme);
        await sleep(200);
        const style = selector => getComputedStyle($(selector));
        const r = {
          text: style("#reading-view p").color,
          link: style("#reading-view a").color,
          mark: style("#reading-view mark").backgroundColor,
          task: style("#reading-view .task-list-checkbox:checked").backgroundColor,
          accent: css("--accent"),
        };
        // 链接和已完成任务框沿用强调色，高亮用浅底色，都与正文色区分。
        if (r.link !== r.task || new Set([r.text, r.link, r.mark]).size !== 3) throw Error(theme + " 行内配色不符 " + JSON.stringify(r));
        return r;
      };
      setDoc(
        "正文里有[链接](https://example.com)和==高亮==。\n\n- [x] 已完成\n- [ ] 未完成\n\n| 列 | 说明 |\n| --- | --- |\n| **粗** | [链接](https://example.com) 与 ==高亮== |\n\n尾",
        0,
      );
      V.dispatch({ selection: { anchor: V.state.doc.length } });
      await blur();
      $("#reading-toggle").click();
      await sleep(400);
      const r = { dark: await colors("dark"), light: await colors("light") };
      $("#reading-toggle").click();
      await sleep(200);
      await blur();
      return r;
    },
    { shot: "e03-accents-light" },
  ),
  page(
    "未保存关闭：取消 / 不保存",
    async () => {
      await setTheme("light");
      V.focus();
      V.dispatch({ changes: { from: V.state.doc.length, insert: "\n未保存的一行" }, userEvent: "input.type" });
      await sleep(400);
      const tabs = $$(".file-tab").length;
      const dirty = T.document().dirty;
      const dot = $(".file-tab.active").textContent;
      $(".file-tab.active .close-tab").click();
      await sleep(200);
      const dlg = $("#unsaved-dialog");
      const shown = dlg.open;
      const text = dlg.textContent;
      dlg.querySelector("button[value=cancel]").click();
      await sleep(200);
      const kept = $$(".file-tab").length === tabs && T.document().name === "日记.md" && doc().includes("未保存的一行");
      $(".file-tab.active .close-tab").click();
      await sleep(200);
      const shown2 = dlg.open;
      dlg.querySelector("button[value=discard]").click();
      await sleep(400);
      const r = { dirty, dot, shown, text, 取消后保留: kept, shown2, tabsBefore: tabs, tabsAfter: $$(".file-tab").length, current: T.document().name };
      if (!dirty || !shown || !kept || r.tabsAfter !== tabs - 1) throw Error("未保存关闭流程不符 " + JSON.stringify(r));
      return r;
    },
    { focus: true },
  ),
  shell("磁盘：不保存后日记.md 未被改写", `cat "${workspace}/笔记/日记.md"; ! grep -q "未保存的一行" "${workspace}/笔记/日记.md"`),
  page("不保存后重新打开：内容为磁盘原文", async () => {
    await openFile("日记.md", "笔记");
    const r = { same: doc() === __orig, dirty: T.document().dirty, doc: doc() };
    if (!r.same || r.dirty) throw Error("草稿未丢弃 " + JSON.stringify(r));
    return r;
  }),
  page(
    "未保存关闭：保存",
    async () => {
      V.focus();
      V.dispatch({ changes: { from: V.state.doc.length, insert: "\n关闭时保存的一行\n" }, userEvent: "input.type" });
      await sleep(400);
      const tabs = $$(".file-tab").length;
      $(".file-tab.active .close-tab").click();
      await sleep(200);
      const dlg = $("#unsaved-dialog");
      const shown = dlg.open;
      dlg.querySelector("button[value=save]").click();
      await sleep(600);
      const r = { shown, tabsBefore: tabs, tabsAfter: $$(".file-tab").length, toast: toast(), status: $("#save-status").textContent };
      // 保存并关闭后标签关掉，不再弹出“当前有其他操作正在处理”。
      if (!shown || r.tabsAfter !== tabs - 1 || r.toast) throw Error("保存并关闭流程不符 " + JSON.stringify(r));
      return r;
    },
    { focus: true },
  ),
  shell("磁盘：保存后日记.md 已写入", `cat "${workspace}/笔记/日记.md"; grep -q "关闭时保存的一行" "${workspace}/笔记/日记.md"`),
];
