import { page, selectFolder, shell } from "./steps.mjs";

// 工作区、文件树、搜索、新建与重命名。
export default ({ workspace, workspace2, out }) => [
  page(
    "初始状态",
    async () => {
      window.__errs = [];
      window.addEventListener("error", e => __errs.push(String(e.message)));
      window.addEventListener("unhandledrejection", e => __errs.push("rejection: " + String(e.reason)));
      const ce = console.error;
      console.error = (...a) => {
        __errs.push(a.map(String).join(" "));
        ce(...a);
      };
      $("#documents-tab").click();
      await sleep(200);
      return { workspace: $(".fb-workspace-choose").textContent, title: $(".fb-workspace-choose").title, rows: rows(), doc: T.document().name };
    },
    { shot: "a01-initial" },
  ),
  selectFolder("后端切换到 full-ws（等同对话框返回该路径）", workspace),
  page(
    "刷新后显示新工作区",
    async () => {
      await refreshWorkspace("full-ws");
      const r = rows();
      if (r.some(x => x.includes("readme.txt") || x.includes(".png"))) throw Error("显示了非 Markdown 文件：" + r);
      return { rows: r, title: $(".fb-workspace-choose").title };
    },
    { shot: "a02-switched" },
  ),
  page(
    "展开多级文件夹",
    async () => {
      const f = name => $$(".fb-tree .fb-node-open").find(b => b.textContent.trim() === name);
      f("笔记").click();
      await sleep(50);
      if (f("笔记").getAttribute("aria-expanded") !== "true") throw Error("未展开");
      f("想法").click();
      await sleep(50);
      const r = rows();
      if (!r.includes("灵感.md") || !r.includes("日记.md")) throw Error("子项缺失 " + r);
      f("空目录").click();
      await sleep(50);
      const empty = $$(".fb-empty").map(e => e.textContent);
      f("空目录").click();
      return { rows: r, empty, selected: $(".fb-selected")?.textContent.trim() };
    },
    { shot: "a03-expanded" },
  ),
  page(
    "搜索：文件名命中",
    async () => {
      const s = $(".fb-search");
      setInput(s, "灵感");
      await sleep(50);
      const r = rows();
      if (r.join() !== "笔记,想法,灵感.md") throw Error("过滤结果不符：" + r);
      return { rows: r };
    },
    { shot: "a04-search-file" },
  ),
  page("搜索：大小写不敏感 + 英文", async () => {
    const s = $(".fb-search");
    setInput(s, "PLAN");
    await sleep(50);
    const r = rows();
    if (!r.includes("Plan-2026.md")) throw Error("大小写不敏感失败：" + r);
    return { rows: r };
  }),
  page("搜索：文件夹名命中显示全部子项", async () => {
    const s = $(".fb-search");
    setInput(s, "项目");
    await sleep(50);
    const r = rows();
    if (!r.includes("Plan-2026.md") || !r.includes("周报.md")) throw Error("文件夹命中未显示子项：" + r);
    return { rows: r };
  }),
  page(
    "搜索：无结果提示",
    async () => {
      const s = $(".fb-search");
      setInput(s, "zzz不存在");
      await sleep(50);
      const r = rows();
      const empty = $$(".fb-tree .fb-empty").map(e => e.textContent);
      if (r.length || !empty[0]?.includes("没有匹配")) throw Error("无结果状态不符 " + r + empty);
      return { empty };
    },
    { shot: "a05-search-empty" },
  ),
  page("搜索：前后空格与清空恢复", async () => {
    const s = $(".fb-search");
    setInput(s, "  日记  ");
    await sleep(50);
    const a = rows();
    setInput(s, "");
    await sleep(50);
    const b = rows();
    if (!a.includes("日记.md")) throw Error("空格未裁剪：" + a);
    if (!b.includes("笔记") || !b.includes("项目") || !b.includes("山中来信.md")) throw Error("清空后未恢复：" + b);
    return { trimmed: a, restored: b, 展开状态保留: b.includes("日记.md") };
  }),
  page("搜索：只按文件名匹配，占位符如实说明", async () => {
    const s = $(".fb-search");
    eq(s.placeholder, "按文件名搜索…", "搜索占位符");
    // “松针”只出现在灵感.md 的正文里。
    setInput(s, "松针");
    await sleep(50);
    const r = rows();
    setInput(s, "");
    if (r.length) throw Error("正文内容不应命中：" + r);
    return { placeholder: s.placeholder };
  }),
  page(
    "点击文件打开",
    async () => {
      const before = $$(".file-tab").length;
      $$(".fb-tree .fb-node-open")
        .find(b => b.textContent.trim() === "日记.md")
        .click();
      await until(() => T.document().name === "日记.md", "打开日记");
      await sleep(150);
      return {
        tabs: $$(".file-tab").map(t => t.textContent),
        doc: T.document().path,
        recent: $$(".fb-recent-list .fb-node-open").map(b => b.textContent),
        heading: $(".md-h1")?.textContent,
        before,
      };
    },
    { shot: "a06-opened" },
  ),
  page("重复打开同一文件不新增标签", async () => {
    const before = $$(".file-tab").length;
    $$(".fb-tree .fb-node-open")
      .find(b => b.textContent.trim() === "日记.md")
      .click();
    await sleep(300);
    const after = $$(".file-tab").length;
    if (after !== before) throw Error("重复打开产生新标签 " + before + "→" + after);
    return { tabs: after };
  }),
  page(
    "新建文件：校验非法输入",
    async () => {
      await wsMenu("在此新建文件…");
      const d = $("dialog.fb-dialog");
      if (!d?.open) throw Error("对话框未打开");
      const input = d.querySelector("input"),
        form = d.querySelector("form"),
        v = () => d.querySelector(".fb-validation").textContent;
      const out = {};
      for (const bad of ["没有扩展名", "../越界.md", "/abs/绝对.md", "a//b.md", "C:evil.md", "back\\slash.md"]) {
        input.value = bad;
        form.requestSubmit();
        await sleep(60);
        out[bad] = v();
        if (!v()) throw Error("未拦截：" + bad);
        if (!$("dialog.fb-dialog")) throw Error("对话框被关闭：" + bad);
      }
      return out;
    },
    { shot: "a07-create-invalid" },
  ),
  page(
    "新建文件：成功并自动打开",
    async () => {
      const d = $("dialog.fb-dialog");
      const input = d.querySelector("input");
      input.value = "笔记/新建测试.md";
      d.querySelector("form").requestSubmit();
      await until(() => T.document().name === "新建测试.md", "新文件打开");
      await sleep(200);
      if ($("dialog.fb-dialog")) throw Error("对话框未关闭");
      return { doc: T.document().path, rows: rows(), content: V.state.doc.toString() };
    },
    { shot: "a08-created" },
  ),
  shell("磁盘确认新文件", `ls -la "${workspace}/笔记/"`),
  page(
    "新建文件：重名报错",
    async () => {
      await wsMenu("在此新建文件…");
      const d = $("dialog.fb-dialog");
      d.querySelector("input").value = "笔记/新建测试.md";
      d.querySelector("form").requestSubmit();
      await sleep(400);
      const msg = d.querySelector(".fb-validation").textContent;
      const open = !!$("dialog.fb-dialog");
      // 对话框开着时，错误只出现在对话框里，不再重复到状态条和全局提示。
      const elsewhere = { toast: toast(), status: $(".fb-status").hidden ? "" : $(".fb-status").textContent };
      d.querySelector("button").click();
      if (!msg || !open) throw Error("重名未在对话框内提示");
      if (elsewhere.toast || elsewhere.status) throw Error("同一条错误重复出现：" + JSON.stringify(elsewhere));
      return { msg, open, ...elsewhere };
    },
    { shot: "a09-create-duplicate" },
  ),
  page("新建文件夹：上级不存在与成功", async () => {
    await wsMenu("在此新建文件夹…");
    let d = $("dialog.fb-dialog");
    d.querySelector("input").value = "不存在/子目录";
    d.querySelector("form").requestSubmit();
    await sleep(400);
    const missing = d.querySelector(".fb-validation").textContent;
    eq(missing, "上级文件夹不存在，请先创建上级文件夹", "上级不存在的提示");
    if (toast() || !$(".fb-status").hidden) throw Error("错误重复出现在对话框之外");
    d.querySelector("input").value = "资料";
    d.querySelector("form").requestSubmit();
    await until(() => !$("dialog.fb-dialog"), "文件夹对话框关闭");
    await sleep(150);
    const r = rows();
    if (!r.includes("资料")) throw Error("文件夹未出现 " + r);
    return { missing, rows: r };
  }),
  page("文件夹内菜单新建（预填路径）", async () => {
    const row = $$(".fb-tree .fb-node-row").find(r => r.querySelector(".fb-node-open").textContent.trim() === "资料");
    row.querySelector(".fb-more").click();
    await sleep(50);
    const labels = $$(".fb-menu [role=menuitem]").map(b => b.textContent);
    $$(".fb-menu [role=menuitem]")
      .find(b => b.textContent === "在此新建文件…")
      .click();
    await sleep(80);
    const d = $("dialog.fb-dialog");
    const prefill = d.querySelector("input").value;
    d.querySelector("input").value = prefill + "清单.md";
    d.querySelector("form").requestSubmit();
    await until(() => T.document().name === "清单.md", "清单打开");
    await sleep(150);
    return { labels, prefill, doc: T.document().path };
  }),
  page(
    "重命名：已打开文档被拦截",
    async () => {
      const row = $$(".fb-tree .fb-node-row").find(r => r.querySelector(".fb-node-open").textContent.trim() === "清单.md");
      row.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 120, clientY: 300 }));
      await sleep(50);
      const menu = $(".fb-menu");
      if (!menu) throw Error("右键菜单未出现");
      const pos = { left: menu.style.left, top: menu.style.top };
      $$(".fb-menu [role=menuitem]")
        .find(b => b.textContent === "重命名…")
        .click();
      await sleep(100);
      const blocked = !$("dialog.fb-dialog");
      const status = $(".fb-status").hidden ? "" : $(".fb-status").textContent;
      if (!blocked || !status.includes("正在打开")) throw Error("未拦截已打开文档重命名 " + status);
      return { pos, status, toast: $("#toast").textContent };
    },
    { shot: "a10-rename-blocked" },
  ),
  page(
    "重命名：未打开文件成功",
    async () => {
      const row = $$(".fb-tree .fb-node-row").find(r => r.querySelector(".fb-node-open").textContent.trim() === "周报.md");
      if (!row) {
        $$(".fb-tree .fb-node-open")
          .find(b => b.textContent.trim() === "项目")
          .click();
        await sleep(60);
      }
      const r2 = $$(".fb-tree .fb-node-row").find(r => r.querySelector(".fb-node-open").textContent.trim() === "周报.md");
      r2.querySelector(".fb-more").click();
      await sleep(50);
      $$(".fb-menu [role=menuitem]")
        .find(b => b.textContent === "重命名…")
        .click();
      await sleep(100);
      const d = $("dialog.fb-dialog");
      const input = d.querySelector("input");
      const pre = input.value;
      input.value = "a/b.md";
      d.querySelector("form").requestSubmit();
      await sleep(60);
      const slash = d.querySelector(".fb-validation").textContent;
      input.value = "周报改名";
      d.querySelector("form").requestSubmit();
      await sleep(60);
      const ext = d.querySelector(".fb-validation").textContent;
      input.value = "周报-改名.md";
      d.querySelector("form").requestSubmit();
      await until(() => !$("dialog.fb-dialog"), "重命名完成");
      await sleep(150);
      const r = rows();
      if (!r.includes("周报-改名.md") || r.includes("周报.md")) throw Error("重命名未生效 " + r);
      return { pre, slash, ext, rows: r };
    },
    { shot: "a11-renamed" },
  ),
  page("重命名文件夹（含已打开文档）被拦截", async () => {
    const row = $$(".fb-tree .fb-node-row").find(r => r.querySelector(".fb-node-open").textContent.trim() === "笔记");
    row.querySelector(".fb-more").click();
    await sleep(50);
    $$(".fb-menu [role=menuitem]")
      .find(b => b.textContent === "重命名…")
      .click();
    await sleep(100);
    const status = $(".fb-status").textContent;
    if ($("dialog.fb-dialog") || !status.includes("包含已打开")) throw Error("未拦截 " + status);
    return { status };
  }),
  page("菜单键盘导航与 Escape", async () => {
    $(".fb-toolbar .fb-more").click();
    await sleep(50);
    const first = document.activeElement.textContent;
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    const second = document.activeElement.textContent;
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true }));
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true }));
    const wrap = document.activeElement.textContent;
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    if ($(".fb-menu")) throw Error("Escape 未关闭");
    if (document.activeElement !== $(".fb-toolbar .fb-more"))
      throw Error("Escape 后焦点未回到触发按钮：" + (document.activeElement.className || document.activeElement.tagName));
    return { first, second, wrap };
  }),
  page(
    "近期文件：点击打开与清空",
    async () => {
      const list = () => $$(".fb-recent-list .fb-node-open").map(b => b.textContent);
      const before = list();
      const target = $$(".fb-recent-list .fb-node-open").find(b => b.textContent === "日记.md");
      target.click();
      await until(() => T.document().name === "日记.md", "近期打开");
      await sleep(150);
      const clear = $(".fb-recent-header button");
      clear.click();
      await until(() => $(".fb-recent-header").hidden, "清空近期");
      return { before, afterHidden: $(".fb-recent-header").hidden, tabs: $$(".file-tab").length };
    },
    { shot: "a12-recent-cleared" },
  ),
  page("根目录折叠后搜索", async () => {
    $(".fb-root-toggle").click();
    await sleep(50);
    const hidden = $(".fb-tree").hidden;
    setInput($(".fb-search"), "灵感");
    await sleep(50);
    const found = !$(".fb-tree").hidden && rows().includes("灵感.md");
    setInput($(".fb-search"), "");
    await sleep(50);
    const collapsedAgain = $(".fb-tree").hidden;
    $(".fb-root-toggle").click();
    if (!hidden || !found || !collapsedAgain) throw Error("折叠根目录后的搜索不符 " + JSON.stringify({ hidden, found, collapsedAgain }));
    return { hidden, found, collapsedAgain };
  }),
  selectFolder("后端切换到 full-ws2", workspace2),
  page(
    "切换工作区后的状态",
    async () => {
      await refreshWorkspace("full-ws2");
      const r = rows();
      const tabs = $$(".file-tab").map(t => t.textContent);
      const doc = T.document();
      V.dispatch({ changes: { from: V.state.doc.length, insert: "\n跨工作区编辑" } });
      await T.flush();
      $("#save-file").click();
      await sleep(600);
      // 已打开的标签不随工作区切换关闭，仍可保存回原位置。
      if (r.length || T.document().dirty || toast()) throw Error("切换工作区后的状态不符 " + JSON.stringify({ r, dirty: T.document().dirty, toast: toast() }));
      return { tabs, currentDoc: doc.path, saveStatus: $("#save-status").textContent };
    },
    { shot: "a13-workspace2" },
  ),
  page("空工作区提示", async () => {
    const empty = $$(".fb-tree .fb-empty").map(e => e.textContent);
    if (empty.length !== 1) throw Error("空工作区应显示一条提示：" + empty);
    return { empty, search: $(".fb-search").disabled };
  }),
  page("没有未捕获的前端错误", async () => {
    // 步骤脚本自身抛错时 WKWebView 会补一条不含细节的 “Script error.”，不属于应用错误。
    const errors = window.__errs.filter(message => message !== "Script error.");
    if (errors.length) throw Error("前端错误：" + errors.join("；"));
    return { ignored: window.__errs.length };
  }),
];
