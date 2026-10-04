import { hover, page, selectFolder, tap } from "./steps.mjs";

// 指针交互：触控板轻触点击（按下与抬起连续到达）在标签、文件导航、状态栏和编辑器里正常响应；
// 标签与文件导航的悬停高亮范围一致，搜索框描边与引用块边线完整。
export default ({ workspace }) => [
  selectFolder("切换到 full-ws", workspace),
  page(
    "刷新并打开两份文档",
    async () => {
      await setTheme("light");
      await refreshWorkspace("full-ws");
      await openFile("山中来信.md");
      await openFile("叶脉笔记.md");
      // 记录页面收到的鼠标事件顺序，轻触时必须是“按下、抬起、点击”。
      window.__pointer = [];
      for (const type of ["mousedown", "mouseup", "click"]) window.addEventListener(type, () => window.__pointer.push(type), true);
      return { tabs: $$(".file-tab-label").map(tab => tab.textContent), current: T.document().name };
    },
    { focus: true },
  ),
  tap("轻触标签：山中来信", () => $$(".file-tab-label").find(tab => tab.textContent.includes("山中来信"))),
  page("轻触标签后切换到山中来信", async () => {
    await until(() => T.document().name === "山中来信.md", "轻触标签未切换文档 " + JSON.stringify(window.__pointer));
    const order = window.__pointer.splice(0).join();
    if (order !== "mousedown,mouseup,click") throw Error("鼠标事件顺序不符 " + order);
    return order;
  }),
  tap("轻触文件导航：叶脉笔记", () => $$(".fb-tree .fb-node-open").find(row => row.textContent.includes("叶脉笔记"))),
  page("轻触文件导航后打开叶脉笔记", async () => {
    await until(() => T.document().name === "叶脉笔记.md", "轻触文件未打开 " + JSON.stringify(window.__pointer));
    const order = window.__pointer.splice(0).join();
    if (order !== "mousedown,mouseup,click") throw Error("鼠标事件顺序不符 " + order);
    return order;
  }),
  tap("轻触状态栏：源码", () => $("#mode-toggle")),
  page("轻触状态栏后进入源码编辑", async () => {
    await until(() => $$(".lm-table-preview").length === 0 && $$(".lm-block-preview").length === 0, "轻触源码按钮未切换模式");
    $("#mode-toggle").click();
    await sleep(200);
    window.__pointer.splice(0);
    return $(".statusbar").textContent;
  }),
  tap("轻触编辑器正文", () => $(".cm-line")),
  page("轻触正文后编辑器获得焦点", async () => {
    await until(() => document.activeElement === V.contentDOM, "轻触正文未聚焦编辑器");
    const order = window.__pointer.splice(0).join();
    if (order !== "mousedown,mouseup,click") throw Error("鼠标事件顺序不符 " + order);
    return order;
  }),
  tap("按压点击标签：山中来信", () => $$(".file-tab-label").find(tab => tab.textContent.includes("山中来信")), 150),
  page("按压点击同样切换文档", async () => {
    await until(() => T.document().name === "山中来信.md", "按压点击未切换文档");
    return window.__pointer.splice(0).join();
  }),
  hover("悬停未选中的标签", () => $$(".file-tab:not(.active) .file-tab-label")[0]),
  page(
    "标签悬停：整个标签一起高亮",
    async () => {
      const tab = $(".file-tab:not(.active):hover");
      if (!tab) throw Error("标签未进入悬停状态");
      const clear = "rgba(0, 0, 0, 0)";
      const r = { tab: getComputedStyle(tab).backgroundColor, inner: [...tab.querySelectorAll("button")].map(button => getComputedStyle(button).backgroundColor) };
      if (r.tab === clear || r.inner.some(color => color !== clear)) throw Error("标签悬停范围不符 " + JSON.stringify(r));
      return r;
    },
    { shot: "f01-tab-hover" },
  ),
  hover("悬停工作区根目录", () => $(".fb-workspace-choose")),
  page(
    "根目录悬停：整行高亮",
    async () => {
      const bar = $(".fb-toolbar:hover");
      if (!bar) throw Error("根目录行未进入悬停状态");
      const clear = "rgba(0, 0, 0, 0)";
      const box = bar.getBoundingClientRect(), row = $(".fb-tree .fb-node-row").getBoundingClientRect();
      const r = { bar: getComputedStyle(bar).backgroundColor, inner: [...bar.querySelectorAll("button")].map(button => getComputedStyle(button).backgroundColor), left: [box.left, row.left], right: [box.right, row.right] };
      if (r.bar === clear || r.inner.some(color => color !== clear)) throw Error("根目录悬停范围不符 " + JSON.stringify(r));
      // 根目录行与文件树的行左右对齐，高亮范围一致。
      if (Math.abs(box.left - row.left) > 0.5 || Math.abs(box.right - row.right) > 0.5) throw Error("根目录行与文件行未对齐 " + JSON.stringify(r));
      window.__rootHover = r.bar;
      return r;
    },
    { shot: "f02-root-hover" },
  ),
  hover("悬停子文件夹", () => node("笔记")),
  page(
    "子文件夹悬停：与根目录同样整行高亮",
    async () => {
      const row = $(".fb-tree .fb-node-row:hover");
      if (!row) throw Error("文件夹行未进入悬停状态");
      const clear = "rgba(0, 0, 0, 0)";
      const r = { row: getComputedStyle(row).backgroundColor, inner: [...row.querySelectorAll("button")].map(button => getComputedStyle(button).backgroundColor), root: window.__rootHover };
      if (r.row !== r.root || r.inner.some(color => color !== clear)) throw Error("文件夹悬停与根目录不一致 " + JSON.stringify(r));
      return r;
    },
    { shot: "f03-folder-hover" },
  ),
  page(
    "搜索框聚焦：描边不被侧栏裁切",
    async () => {
      $(".fb-search").focus();
      await sleep(120);
      const shadow = getComputedStyle($(".fb-search-box")).boxShadow;
      if (!shadow.includes("inset")) throw Error("搜索框描边会被滚动容器裁切 " + shadow);
      return shadow;
    },
    { shot: "f04-search-focus" },
  ),
  page(
    "引用块：连续的引用行连成一整块",
    async () => {
      $("#new-file").click();
      await until(() => !T.document().path && doc() === "", "新建空白文档");
      setDoc("> 第一行\n> 第二行\n> 第三行\n\n正文", 0);
      V.dispatch({ selection: { anchor: V.state.doc.length } });
      await blur();
      const lines = $$(".cm-line.md-quote").map(line => { const style = getComputedStyle(line); return [style.borderTopLeftRadius, style.borderBottomLeftRadius]; });
      if (JSON.stringify(lines) !== JSON.stringify([["4px", "0px"], ["0px", "0px"], ["0px", "4px"]])) throw Error("引用行圆角不符 " + JSON.stringify(lines));
      return lines;
    },
    { shot: "f05-quote" },
  ),
];
