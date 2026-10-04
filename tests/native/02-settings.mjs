import { page, selectFolder, shell } from "./steps.mjs";

// 设置页、外观、快捷键与持久化。
export default ({ workspace, workspace2, out }) => [
  page("初始设置与存储", async () => {
    return {
      stored: stored(),
      shortcuts: localStorage.getItem("leafmark-shortcuts-v1"),
      theme: document.documentElement.dataset.theme,
      keys: Object.keys(localStorage),
    };
  }),
  page("恢复默认作为起点", async () => {
    const s = await resetSettings();
    if (s.fontSize !== 15 || s.font !== "newsreader" || s.readingWidth !== "comfort") throw Error("未恢复 " + JSON.stringify(s));
    // 默认值：字号 15、行高 1.4、自动保存开启。
    if (s.lineHeight !== 1.4 || s.autoSave !== true) throw Error("默认值不符 " + JSON.stringify(s));
    return s;
  }),
  page(
    "经“更多操作”菜单打开设置",
    async () => {
      $("#more-actions").click();
      await sleep(60);
      const labels = $$(".action-menu button").map(b => b.textContent);
      $$(".action-menu button")
        .find(b => b.textContent === "设置…")
        .click();
      await sleep(120);
      if (!S()) throw Error("设置未打开");
      return {
        labels,
        heading: $(".settings-header h3").textContent,
        tabs: $$(".settings-tab").map(t => t.textContent),
        focus: document.activeElement.textContent,
      };
    },
    { shot: "b01-editor-panel" },
  ),
  page("正文字体四个选项", async () => {
    await openS();
    const sel = rowOf("正文字体").querySelector("select");
    const out = {};
    for (const v of ["serif", "sans", "mono", "newsreader"]) {
      change(sel, v);
      await sleep(40);
      out[v] = { book: css("--book").slice(0, 40), computed: getComputedStyle($(".cm-content")).fontFamily.slice(0, 40), stored: stored().font };
      if (stored().font !== v) throw Error("字体未保存 " + v);
    }
    change(sel, "evil");
    await sleep(40);
    out.invalid = { stored: stored().font, selectValue: sel.value };
    change(sel, "newsreader");
    return out;
  }),
  page("字号步进、边界与非法输入", async () => {
    await openS();
    const row = rowOf("字号");
    const [minus, plus] = [row.querySelector("[aria-label=减小字号]"), row.querySelector("[aria-label=增大字号]")];
    const input = row.querySelector("input");
    const out = { start: input.value };
    plus.click();
    await sleep(30);
    out.plus = [input.value, css("--editor-size")];
    minus.click();
    minus.click();
    await sleep(30);
    out.minus = [input.value, css("--editor-size")];
    change(input, "99");
    await sleep(30);
    out.max = [input.value, plus.disabled, css("--editor-size")];
    change(input, "5");
    await sleep(30);
    out.min = [input.value, minus.disabled, css("--editor-size")];
    change(input, "");
    await sleep(30);
    out.empty = [input.value, stored().fontSize];
    change(input, "15.5");
    await sleep(30);
    out.fraction = [input.value, stored().fontSize, css("--editor-size")];
    change(input, "-3");
    await sleep(30);
    out.negative = [input.value, stored().fontSize];
    change(input, "15");
    await sleep(30);
    out.end = [input.value, stored().fontSize];
    if (out.max[0] !== "28" || !out.max[1] || out.min[0] !== "12" || !out.min[1]) throw Error("边界不符 " + JSON.stringify(out));
    // 字号只取整数像素：小数就近取整，输入框、存储和实际字号一致。
    if (out.fraction.join() !== "16,16,16px") throw Error("小数字号未取整 " + JSON.stringify(out.fraction));
    if (out.empty[1] !== 12 || out.negative[1] !== 12 || out.end[1] !== 15) throw Error("非法输入处理不符 " + JSON.stringify(out));
    return out;
  }),
  page("行高与阅读宽度", async () => {
    await openS();
    const lh = rowOf("行高").querySelector("select");
    const options = [...lh.options].map(o => o.value);
    change(lh, "2");
    await sleep(30);
    const out = { options, lh: [css("--editor-line-height"), stored().lineHeight, getComputedStyle($(".cm-line")).lineHeight] };
    change(lh, "1.4");
    out.widths = {};
    for (const [t, k] of [
      ["窄", "narrow"],
      ["宽", "wide"],
      ["舒适", "comfort"],
    ]) {
      seg("阅读宽度", t).click();
      await sleep(30);
      out.widths[k] = [css("--reading-width"), seg("阅读宽度", t).getAttribute("aria-pressed"), stored().readingWidth];
    }
    return out;
  }),
  page(
    "阅读宽度：宽",
    async () => {
      await openS();
      seg("阅读宽度", "宽").click();
      await closeS();
      await sleep(150);
      const w = $(".cm-content").getBoundingClientRect().width;
      return { contentWidth: w, inner: innerWidth };
    },
    { shot: "b02-width-wide" },
  ),
  page(
    "阅读宽度：窄 + 字号 24 + 等宽字体",
    async () => {
      await openS();
      seg("阅读宽度", "窄").click();
      change(rowOf("字号").querySelector("input"), "24");
      change(rowOf("正文字体").querySelector("select"), "mono");
      await closeS();
      await sleep(200);
      return {
        contentWidth: $(".cm-content").getBoundingClientRect().width,
        size: getComputedStyle($(".cm-line")).fontSize,
        h1: getComputedStyle($(".md-h1") || $(".cm-line")).fontSize,
      };
    },
    { shot: "b03-narrow-mono-24" },
  ),
  page("恢复默认设置按钮", async () => {
    await openS();
    $(".settings-footer .settings-reset").click();
    await sleep(80);
    const s = stored();
    const ui = {
      font: rowOf("正文字体").querySelector("select").value,
      size: rowOf("字号").querySelector("input").value,
      width: seg("阅读宽度", "舒适").getAttribute("aria-pressed"),
    };
    await closeS();
    if (s.fontSize !== 15 || s.font !== "newsreader" || s.readingWidth !== "comfort") throw Error("未恢复 " + JSON.stringify(s));
    if (ui.font !== "newsreader" || ui.size !== "15" || ui.width !== "true") throw Error("界面未同步恢复 " + JSON.stringify(ui));
    return { s, ui };
  }),
  page("实时渲染开关与状态栏同步", async () => {
    await openS();
    const t = rowOf("实时渲染").querySelector("input");
    const before = t.checked;
    t.click();
    await sleep(60);
    await closeS();
    await sleep(150);
    const off = {
      mode: $("#edit-mode").textContent,
      toggle: $("#mode-toggle").textContent,
      previews: $$(".lm-block-preview").length,
      strong: $$(".md-strong").length,
      stored: stored().liveRendering,
    };
    $("#mode-toggle").click();
    await sleep(150);
    await openS();
    const synced = rowOf("实时渲染").querySelector("input").checked;
    await closeS();
    if (off.mode !== "源码编辑" || off.previews !== 0) throw Error("关闭实时渲染无效 " + JSON.stringify(off));
    if (!synced) throw Error("状态栏切换未同步到设置开关");
    return { before, off, synced, on: { mode: $("#edit-mode").textContent, previews: $$(".lm-block-preview").length } };
  }),
  page("显示当前语法开关", async () => {
    const text = V.state.doc.toString();
    const pos = text.indexOf("加粗文字");
    const probe = async () => {
      V.focus();
      V.dispatch({ selection: { anchor: pos + 1 } });
      await sleep(150);
      const line = V.domAtPos(pos).node.parentElement.closest(".cm-line");
      return { focus: V.hasFocus, lineText: line.textContent };
    };
    const on = await probe();
    await openS();
    rowOf("显示当前语法").querySelector("input").click();
    await closeS();
    const off = await probe();
    await openS();
    rowOf("显示当前语法").querySelector("input").click();
    await closeS();
    return { on, off };
  }),
  page(
    "外观：深色",
    async () => {
      await openS();
      await tab("外观");
      seg("外观模式", "深色").click();
      await sleep(120);
      const r = {
        theme: document.documentElement.dataset.theme,
        stored: stored().theme,
        pressed: seg("外观模式", "深色").getAttribute("aria-pressed"),
        legacy: localStorage.getItem("leafmark-theme"),
        names: $$(".settings-theme-name").map(n => n.textContent),
      };
      if (r.theme !== "dark") throw Error("深色未生效");
      if (r.names.join() !== "Leafmark 浅色,Leafmark 深色") throw Error("主题名称不符 " + JSON.stringify(r.names));
      return r;
    },
    { shot: "b04-appearance-dark" },
  ),
  page(
    "深色下的编辑页",
    async () => {
      await closeS();
      await sleep(150);
      return { bg: getComputedStyle(document.body).backgroundColor, color: getComputedStyle($(".cm-content")).color };
    },
    { shot: "b05-editor-dark" },
  ),
  page(
    "外观：跟随系统与浅色",
    async () => {
      await openS();
      await tab("外观");
      seg("外观模式", "跟随系统").click();
      await sleep(100);
      const sys = { theme: document.documentElement.dataset.theme, prefersDark: matchMedia("(prefers-color-scheme: dark)").matches, stored: stored().theme };
      seg("外观模式", "浅色").click();
      await sleep(100);
      const light = { theme: document.documentElement.dataset.theme, stored: stored().theme };
      if ((sys.theme === "dark") !== sys.prefersDark) throw Error("跟随系统不一致");
      return { sys, light };
    },
    { shot: "b06-appearance-light" },
  ),
  page("标题栏主题按钮与设置同步", async () => {
    await closeS();
    const visible = getComputedStyle($("#theme-toggle")).display !== "none" && $("#theme-toggle").getBoundingClientRect().width > 0;
    $("#theme-toggle").click();
    await sleep(80);
    const t1 = document.documentElement.dataset.theme;
    await openS();
    await tab("外观");
    const pressed = seg("外观模式", "深色").getAttribute("aria-pressed");
    seg("外观模式", "浅色").click();
    await sleep(60);
    await closeS();
    return { themeButtonVisible: visible, afterToggle: t1, settingsShowsDark: pressed, stored: stored().theme };
  }),
  page("分类键盘导航", async () => {
    await openS();
    const tabs = $$(".settings-tab");
    tabs[0].click();
    tabs[0].focus();
    const seq = [];
    const key = k => {
      document.activeElement.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true }));
      seq.push(k + "→" + $(".settings-header h3").textContent + "/" + document.activeElement.textContent);
    };
    key("ArrowDown");
    key("ArrowDown");
    key("End");
    key("ArrowDown");
    key("ArrowUp");
    key("Home");
    return seq;
  }),
  page("个性化入口跳转", async () => {
    await openS();
    await tab("编辑器");
    rowOf("外观").querySelector("button").click();
    await sleep(60);
    const a = $(".settings-header h3").textContent;
    await tab("编辑器");
    const label = rowOf("外观").querySelector("button").textContent;
    rowOf("快捷键").querySelector("button").click();
    await sleep(60);
    if (!label.includes("Leafmark · ") || /ember/i.test($(".leafmark-settings").textContent)) throw Error("主题名称不符 " + label);
    return { a, b: $(".settings-header h3").textContent, label };
  }),
  page(
    "快捷键面板",
    async () => {
      await openS();
      await tab("快捷键");
      return { list: $$(".settings-shortcut").map(e => e.textContent), intro: panel().querySelector(".settings-intro").textContent };
    },
    { shot: "b07-shortcuts-panel" },
  ),
  page(
    "自定义快捷键对话框：录制、保留键、冲突、应用",
    async () => {
      await openS();
      await tab("快捷键");
      $(".settings-customize-shortcuts").click();
      await sleep(150);
      const d = $("dialog.leafmark-shortcuts");
      if (!d?.open) throw Error("快捷键对话框未打开");
      if (S()) throw Error("设置页未关闭");
      const sel = d.querySelector("select"),
        rec = d.querySelector(".shortcut-recorder"),
        apply = d.querySelector(".shortcut-primary"),
        st = () => d.querySelector(".shortcut-status").textContent;
      const press = o => rec.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...o }));
      const out = { actions: [...sel.options].map(o => o.textContent), current: d.querySelector(".shortcut-current").textContent };
      rec.click();
      out.recording = rec.textContent;
      press({ key: "j", code: "KeyJ", metaKey: true });
      out.reserved = st();
      press({ key: "y", code: "KeyY" });
      out.bare = st();
      press({ key: "o", code: "KeyO", metaKey: true });
      out.conflict = st();
      press({ key: "Y", code: "KeyY", metaKey: true, shiftKey: true });
      out.valid = [st(), rec.textContent, apply.disabled];
      apply.click();
      await sleep(60);
      out.applied = [st(), d.querySelector(".shortcut-current").textContent, localStorage.getItem("leafmark-shortcuts-v1")];
      if (!out.reserved || !out.bare || !out.conflict.includes("打开文件") || out.valid[2] || out.applied[1] !== "当前绑定：Mod-Shift-y") {
        throw Error("快捷键录制流程不符 " + JSON.stringify(out));
      }
      // 下拉框与设置页一致：去掉系统外观，使用自绘箭头。
      const select = getComputedStyle(sel);
      if (select.appearance !== "none" || select.backgroundImage === "none") throw Error("下拉框样式与设置页不一致 " + select.appearance);
      return out;
    },
    { shot: "b08-shortcut-dialog" },
  ),
  page("新快捷键生效：Mod-Shift-y 触发保存", async () => {
    const d = $("dialog.leafmark-shortcuts");
    d.querySelectorAll(".shortcut-actions button")[2].click();
    await sleep(120);
    const closed = !$("dialog.leafmark-shortcuts");
    const focus = document.activeElement === V.contentDOM;
    V.dispatch({ changes: { from: V.state.doc.length, insert: "\n快捷键保存" } });
    await T.flush();
    const dirty = T.document().dirty;
    V.contentDOM.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Y", code: "KeyY", keyCode: 89, metaKey: true, shiftKey: true, bubbles: true, cancelable: true }),
    );
    await sleep(700);
    const afterNew = T.document().dirty;
    V.dispatch({ changes: { from: V.state.doc.length, insert: "!" } });
    await T.flush();
    const ev = new KeyboardEvent("keydown", { key: "s", code: "KeyS", keyCode: 83, metaKey: true, bubbles: true, cancelable: true });
    V.contentDOM.dispatchEvent(ev);
    await sleep(700);
    await $("#settings-toggle").click();
    await sleep(80);
    await tab("快捷键");
    const list = $$(".settings-shortcut").map(e => e.textContent);
    await closeS();
    return { closed, focus, dirty, 新键保存后dirty: afterNew, 旧键ModS后dirty: T.document().dirty, 旧键被拦截: ev.defaultPrevented, list };
  }),
  page("快捷键恢复默认", async () => {
    $("#format-menu-toggle").click();
    await sleep(50);
    $$(".action-menu button")
      .find(b => b.textContent === "自定义快捷键…")
      .click();
    await sleep(120);
    const d = $("dialog.leafmark-shortcuts");
    d.querySelectorAll(".shortcut-actions button")[0].click();
    await sleep(60);
    const st = d.querySelector(".shortcut-status").textContent;
    const cur = d.querySelector(".shortcut-current").textContent;
    d.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
    await sleep(80);
    $("#save-file").click();
    await sleep(500);
    return { st, cur, closed: !$("dialog.leafmark-shortcuts"), saved: !T.document().dirty };
  }),
  page(
    "通用：专注模式",
    async () => {
      await openS();
      await tab("通用");
      rowOf("专注模式").querySelector("input").click();
      await sleep(60);
      await closeS();
      V.focus();
      V.dispatch({ selection: { anchor: V.state.doc.toString().indexOf("加粗文字") } });
      await sleep(200);
      const app = $("#app");
      return {
        focusClass: app.classList.contains("focus-mode"),
        sidebarHidden: app.classList.contains("sidebar-hidden"),
        showButton: !$("#show-sidebar").hidden,
      };
    },
    { shot: "b09-focus-mode" },
  ),
  page("专注模式下展开侧栏再关闭", async () => {
    $("#show-sidebar").click();
    await sleep(80);
    const shown = !$("#app").classList.contains("sidebar-hidden");
    await openS();
    await tab("通用");
    rowOf("专注模式").querySelector("input").click();
    await sleep(60);
    await closeS();
    return { 专注模式中可展开侧栏: shown, 关闭后侧栏: !$("#app").classList.contains("sidebar-hidden"), focusClass: $("#app").classList.contains("focus-mode") };
  }),
  page("通用：打字机模式", async () => {
    await openS();
    await tab("通用");
    rowOf("打字机模式").querySelector("input").click();
    await closeS();
    V.focus();
    const end = V.state.doc.length;
    V.dispatch({ selection: { anchor: end } });
    await sleep(250);
    const c = V.coordsAtPos(end),
      r = V.scrollDOM.getBoundingClientRect();
    const out = {
      cls: $("#app").classList.contains("typewriter-mode"),
      caretY: Math.round(c.top),
      viewportMid: Math.round(r.top + r.height / 2),
      scrollTop: V.scrollDOM.scrollTop,
    };
    await openS();
    await tab("通用");
    rowOf("打字机模式").querySelector("input").click();
    await closeS();
    return out;
  }),
  page(
    "通用：清理草稿与历史的确认流程",
    async () => {
      await openS();
      await tab("通用");
      const out = {};
      for (const label of ["清理恢复草稿", "清理历史记录"]) {
        const btn = rowOf(label).querySelector("button");
        const conf = rowOf(label).nextElementSibling;
        btn.click();
        await sleep(40);
        const shown = !conf.hidden;
        [...conf.querySelectorAll("button")].find(b => b.textContent === "取消").click();
        await sleep(40);
        const cancelled = conf.hidden;
        btn.click();
        [...conf.querySelectorAll("button")].find(b => b.textContent === "确认清理").click();
        await sleep(400);
        out[label] = { disabled: btn.disabled, shown, cancelled, status: $(".settings-status").textContent, hiddenAfter: conf.hidden };
      }
      for (const [label, item] of Object.entries(out)) {
        if (!item.shown || !item.cancelled || !item.hiddenAfter || !item.status.includes("完成")) throw Error(label + " 流程不符 " + JSON.stringify(item));
      }
      // 软件更新：版本说明与按钮同在一行；“安装更新”只在有可用更新时出现，并排在“检查更新”右侧。
      const row = rowOf("Leafmark");
      const buttons = [...row.querySelectorAll(".settings-update-actions button")];
      eq(buttons.map(button => button.textContent).join(), "检查更新,安装更新", "更新按钮");
      const [check, install] = buttons.map(button => button.getBoundingClientRect());
      const description = row.querySelector(".settings-description").getBoundingClientRect();
      if (check.left < description.right || check.bottom < description.top || check.top > description.bottom)
        throw Error("检查更新未与版本说明同行 " + JSON.stringify({ check, description }));
      if (install.width && (Math.abs(check.top - install.top) > 1 || install.left < check.right))
        throw Error("更新按钮未并排 " + JSON.stringify({ check, install }));
      out.updates = { version: row.querySelector(".settings-description").textContent, installVisible: install.width > 0 };
      return out;
    },
    { shot: "b10-general" },
  ),
  page("设置页内 ⌘S/⌘N 不穿透", async () => {
    await openS();
    const tabs = $$(".file-tab").length;
    for (const k of ["s", "n", "o"]) {
      const e = new KeyboardEvent("keydown", { key: k, metaKey: true, bubbles: true, cancelable: true });
      document.activeElement.dispatchEvent(e);
    }
    await sleep(300);
    const r = { tabsBefore: tabs, tabsAfter: $$(".file-tab").length, dialog: !!$("dialog[open]") };
    S().querySelector(".settings-back").click();
    await sleep(80);
    if (r.tabsBefore !== r.tabsAfter) throw Error("快捷键穿透");
    return { ...r, closedByBack: !S() };
  }),
  page("持久化：改设置后重载", async () => {
    await openS();
    await tab("编辑器");
    change(rowOf("字号").querySelector("input"), "21");
    seg("阅读宽度", "宽").click();
    rowOf("自动保存").querySelector("input").click();
    await tab("外观");
    seg("外观模式", "深色").click();
    await sleep(80);
    await closeS();
    return stored();
  }),
  page(
    "重载页面",
    async () => {
      setTimeout(() => location.reload(), 50);
      return true;
    },
    { sleep: 2500 },
  ),
  page(
    "重载后设置仍生效",
    async () => {
      await until(() => window.leafmarkVerification && T.document().path, "重载");
      await sleep(300);
      const r = { size: css("--editor-size"), width: css("--reading-width"), theme: document.documentElement.dataset.theme, stored: stored() };
      if (r.size !== "21px" || r.theme !== "dark" || r.width !== "1040px") throw Error("重载后丢失 " + JSON.stringify(r));
      return r;
    },
    { shot: "b11-after-reload" },
  ),
  page("损坏的本地存储回退默认", async () => {
    localStorage.setItem("leafmark-settings", '{"fontSize":"abc","font":"x","theme":7,"lineHeight":99,"extra":1');
    return true;
  }),
  page(
    "重载页面（损坏设置）",
    async () => {
      setTimeout(() => location.reload(), 50);
      return true;
    },
    { sleep: 2500 },
  ),
  page("损坏设置后的状态并清理", async () => {
    await until(() => window.leafmarkVerification && T.document().path, "重载");
    await sleep(300);
    const r = { size: css("--editor-size"), lh: css("--editor-line-height"), stored: localStorage.getItem("leafmark-settings") };
    if (r.size !== "15px" || r.lh !== "1.4") throw Error("损坏的设置未回退默认 " + JSON.stringify(r));
    const final = await resetSettings();
    localStorage.removeItem("leafmark-shortcuts-v1");
    return { ...r, final };
  }),
];
