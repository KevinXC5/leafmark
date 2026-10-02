import "./settings.css";
import { createElement, SquarePen, Sun, Command, SlidersHorizontal, ChevronRight, Check, X, Monitor } from "lucide";

export type Settings = {
  font: "newsreader" | "serif" | "sans" | "mono";
  fontSize: number;
  lineHeight: number;
  readingWidth: "narrow" | "comfort" | "wide";
  liveRendering: boolean;
  showActiveSyntax: boolean;
  autoSave: boolean;
  theme: "light" | "dark" | "system";
  focusMode: boolean;
  typewriter: boolean;
};

export const defaultSettings: Readonly<Settings> = Object.freeze({
  font: "newsreader", fontSize: 18, lineHeight: 1.6, readingWidth: "comfort",
  liveRendering: true, showActiveSyntax: true, autoSave: false, theme: "system",
  focusMode: false, typewriter: false,
});

export const SETTINGS_STORAGE_KEY = "leafmark-settings";
export const AUTO_SAVE_DELAY = 2000;
export const editorFonts: Record<Settings["font"], string> = {
  newsreader: '"Newsreader", "Songti SC", "Noto Serif CJK SC", Georgia, serif',
  serif: '"Songti SC", "Noto Serif CJK SC", Georgia, serif',
  sans: '-apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans CJK SC", sans-serif',
  mono: '"Geist Mono", "SFMono-Regular", Consolas, monospace',
};
export const readingWidths: Record<Settings["readingWidth"], number> = { narrow: 600, comfort: 856, wide: 1040 };

/** 逐字段校验，不信任本地存储；未知字段不会进入运行时设置。 */
export function validateSettings(value: unknown): Settings {
  const result: Settings = { ...defaultSettings };
  if (typeof value !== "object" || value === null || Array.isArray(value)) return result;
  const source = value as Record<string, unknown>;
  for (const key of ["font", "readingWidth", "theme"] as const) {
    const allowed = key === "font" ? ["newsreader", "serif", "sans", "mono"]
      : key === "readingWidth" ? ["narrow", "comfort", "wide"] : ["light", "dark", "system"];
    if (typeof source[key] === "string" && allowed.includes(source[key])) {
      Object.assign(result, { [key]: source[key] });
    }
  }
  if (typeof source.fontSize === "number" && Number.isFinite(source.fontSize) && source.fontSize >= 12 && source.fontSize <= 28) result.fontSize = source.fontSize;
  if (typeof source.lineHeight === "number" && Number.isFinite(source.lineHeight) && source.lineHeight >= 1.3 && source.lineHeight <= 2.2) result.lineHeight = source.lineHeight;
  for (const key of ["liveRendering", "showActiveSyntax", "autoSave", "focusMode", "typewriter"] as const) {
    if (typeof source[key] === "boolean") result[key] = source[key];
  }
  return result;
}

export function loadSettings(): Settings {
  try {
    const stored = localStorage.getItem(SETTINGS_STORAGE_KEY);
    return validateSettings(stored === null ? null : JSON.parse(stored));
  } catch {
    return { ...defaultSettings };
  }
}

/** 存储被禁用或空间不足时，仍允许本次会话继续调整设置。 */
export function saveSettings(settings: Settings): void {
  try { localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(validateSettings(settings))); } catch { /* 保留会话内设置。 */ }
}

export type SettingsOptions = {
  onClearDrafts?: () => void | Promise<void>;
  onClearHistory?: () => void | Promise<void>;
  onCustomizeShortcuts?: () => void;
  updates?: {
    status: () => Promise<{ version: string; enabled: boolean; available: string; notes: string; installed: boolean }>;
    check: () => Promise<{ version: string; enabled: boolean; available: string; notes: string; installed: boolean }>;
    install: () => Promise<{ version: string; enabled: boolean; available: string; notes: string; installed: boolean }>;
  };
  /** 集成方可提供实际预设，设置页仅展示，不修改绑定。 */
  shortcuts?: readonly { label: string; keys: string }[];
};

let activeDialog: HTMLDialogElement | null = null;
let dialogSequence = 0;

function node<K extends keyof HTMLElementTagNameMap>(tag: K, className = "", text?: string): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag);
  element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

/** 变更即保存并回传快照；编辑器重配置与两秒自动保存由集成方执行。 */
export function openSettings(settings: Settings, onChange: (settings: Settings) => void, options: SettingsOptions = {}): void {
  if (activeDialog?.open) { activeDialog.focus(); return; }
  let current = validateSettings(settings);
  let afterClose: (() => void) | undefined;
  const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const prefix = `leafmark-settings-${++dialogSequence}`;
  const dialog = node("dialog", "leafmark-settings");
  activeDialog = dialog;
  dialog.setAttribute("aria-labelledby", `${prefix}-title`);
  const side = node("aside", "settings-sidebar");
  const title = node("h2", "settings-title", "设置");
  title.id = `${prefix}-title`;
  side.append(node("span", "settings-brand", "Leafmark"), title);
  const tabs = node("div", "settings-tabs");
  tabs.setAttribute("role", "tablist");
  tabs.setAttribute("aria-label", "设置分类");
  const sidebarNote = node("div", "settings-sidebar-note");
  sidebarNote.append(node("p", "", "留一点空间，给书写。"), node("span", "", "Leafmark"));
  const back = node("button", "settings-back");
  back.type = "button";
  back.append(node("kbd", "", "Esc"), node("span", "", "返回书写"));
  back.addEventListener("click", () => dialog.close());
  side.append(tabs, sidebarNote, back);
  const main = node("div", "settings-main");
  const header = node("header", "settings-header");
  const heading = node("h3");
  const close = node("button", "settings-close");
  close.setAttribute("aria-label", "返回书写");
  close.append(createElement(X, { width: 18, height: 18, "aria-hidden": "true" }));
  close.type = "button";
  close.title = "返回书写（Escape）";
  close.addEventListener("click", () => dialog.close());
  header.append(heading, close);
  const content = node("div", "settings-content");
  const footer = node("footer", "settings-footer");
  const status = node("span", "settings-status", "调整即时生效");
  status.setAttribute("role", "status");
  const reset = node("button", "settings-reset", "恢复默认设置");
  reset.type = "button";
  const saved = node("span", "settings-saved");
  saved.append(createElement(Check, { width: 13, height: 13, "aria-hidden": "true" }), status);
  footer.append(saved, reset);
  main.append(header, content, footer);
  dialog.append(side, main);

  const bindings: (() => void)[] = [];
  let controlSequence = 0;
  const publish = () => {
    saveSettings(current);
    onChange({ ...current });
    status.textContent = "调整即时生效";
  };
  function row(parent: HTMLElement, label: string, description: string, control: HTMLElement) {
    const wrapper = node("div", "settings-row");
    const copy = node("div", "settings-row-copy");
    const name = node("label", "settings-label", label);
    control.id = `${prefix}-control-${++controlSequence}`;
    name.htmlFor = control.id;
    copy.append(name);
    if (description) copy.append(node("p", "settings-description", description));
    else wrapper.classList.add("settings-row-compact");
    if (!control.matches("input, select, button")) {
      name.removeAttribute("for");
      name.id = `${control.id}-label`;
      control.setAttribute("aria-labelledby", name.id);
    }
    wrapper.append(copy, control);
    parent.append(wrapper);
  }
  function toggle(parent: HTMLElement, key: "liveRendering" | "showActiveSyntax" | "autoSave" | "focusMode" | "typewriter", label: string, description: string) {
    const input = node("input", "settings-toggle");
    input.type = "checkbox";
    input.setAttribute("role", "switch");
    row(parent, label, description, input);
    const refresh = () => { input.checked = current[key]; };
    bindings.push(refresh); refresh();
    input.addEventListener("change", () => { current[key] = input.checked; publish(); });
  }
  function select<K extends "font" | "readingWidth" | "theme">(parent: HTMLElement, key: K, label: string, description: string, choices: readonly (readonly [Settings[K], string])[]) {
    const input = node("select", "settings-select");
    for (const [value, text] of choices) { const option = node("option", "", text); option.value = value; input.append(option); }
    row(parent, label, description, input);
    const refresh = () => { input.value = current[key]; };
    bindings.push(refresh); refresh();
    input.addEventListener("change", () => {
      // 仅接受选项中的值，避免 DOM 被修改后传入任意字符串。
      const choice = choices.find(([value]) => value === input.value);
      if (choice) { current[key] = choice[0]; publish(); }
    });
  }
  function section(parent: HTMLElement, title: string): HTMLElement {
    const group = node("section", "settings-group");
    const body = node("div", "settings-group-body");
    group.append(node("h4", "settings-group-title", title), body);
    parent.append(group);
    return body;
  }
  function segmented<K extends "readingWidth" | "theme">(parent: HTMLElement, key: K, label: string, choices: readonly (readonly [Settings[K], string])[]) {
    const group = node("div", `settings-segmented settings-segmented-${key}`);
    group.setAttribute("role", "group");
    const buttons = choices.map(([value, text]) => {
      const button = node("button", "", text); button.type = "button";
      button.addEventListener("click", () => { current[key] = value; refresh(); publish(); });
      group.append(button);
      return { value, button };
    });
    const refresh = () => buttons.forEach(({ value, button }) => button.setAttribute("aria-pressed", String(current[key] === value)));
    bindings.push(refresh); refresh();
    row(parent, label, "", group);
  }
  function fontSize(parent: HTMLElement) {
    const group = node("div", "settings-stepper");
    const minus = node("button", "", "−"); minus.type = "button"; minus.setAttribute("aria-label", "减小字号");
    const input = node("input", "settings-number"); input.type = "number"; input.min = "12"; input.max = "28"; input.step = "1"; input.setAttribute("aria-label", "字号");
    const plus = node("button", "", "+"); plus.type = "button"; plus.setAttribute("aria-label", "增大字号");
    group.append(minus, input, node("span", "", "px"), plus);
    row(parent, "字号", "", group);
    const refresh = () => { input.value = String(current.fontSize); minus.disabled = current.fontSize <= 12; plus.disabled = current.fontSize >= 28; };
    const update = (value: number) => { if (Number.isFinite(value)) { current.fontSize = Math.max(12, Math.min(28, value)); publish(); } refresh(); };
    minus.addEventListener("click", () => update(current.fontSize - 1));
    plus.addEventListener("click", () => update(current.fontSize + 1));
    input.addEventListener("change", () => update(input.valueAsNumber));
    bindings.push(refresh); refresh();
  }
  function lineHeight(parent: HTMLElement) {
    const input = node("select", "settings-select settings-line-height");
    // 保留 API 接受的自定义行高，避免打开设置时悄悄覆盖原值。
    const values = [...new Set([current.lineHeight, defaultSettings.lineHeight, ...Array.from({ length: 19 }, (_, i) => Math.round((1.3 + i * .05) * 100) / 100)])].sort((a, b) => a - b);
    for (const value of values) { const option = node("option", "", String(value)); option.value = String(value); input.append(option); }
    row(parent, "行高", "", input);
    const refresh = () => { input.value = String(current.lineHeight); };
    bindings.push(refresh); refresh();
    input.addEventListener("change", () => { current.lineHeight = Number(input.value); publish(); });
  }
  function destination(parent: HTMLElement, label: string, description: string, text: string, index: number, swatch = false) {
    const button = node("button", "settings-destination"); button.type = "button";
    if (swatch) button.append(node("span", "settings-swatch"));
    button.append(node("span", "", text), createElement(ChevronRight, { width: 16, height: 16, "aria-hidden": "true" }));
    row(parent, label, description, button);
    button.addEventListener("click", () => activate(index, true));
    return button;
  }

  const panels: { tab: HTMLButtonElement; panel: HTMLElement; label: string }[] = [];
  for (const [id, label] of [["editor", "编辑器"], ["appearance", "外观"], ["shortcuts", "快捷键"], ["general", "通用"]] as const) {
    const tab = node("button", "settings-tab");
    const icons = { editor: SquarePen, appearance: Sun, shortcuts: Command, general: SlidersHorizontal };
    tab.append(createElement(icons[id], { width: 18, height: 18, "aria-hidden": "true" }), node("span", "", label));
    tab.type = "button"; tab.id = `${prefix}-tab-${id}`; tab.setAttribute("role", "tab");
    const panel = node("section", "settings-panel");
    panel.id = `${prefix}-panel-${id}`; panel.setAttribute("role", "tabpanel");
    panel.setAttribute("aria-labelledby", tab.id); tab.setAttribute("aria-controls", panel.id);
    panels.push({ tab, panel, label }); tabs.append(tab); content.append(panel);
    if (id === "editor") {
      panel.append(node("p", "settings-intro", "为自己的书写节奏，留一点空间。"));
      const typography = section(panel, "排版与布局");
      select(typography, "font", "正文字体", "", [["newsreader", "Newsreader"], ["serif", "系统衬线"], ["sans", "系统无衬线"], ["mono", "等宽"]]);
      fontSize(typography);
      lineHeight(typography);
      segmented(typography, "readingWidth", "阅读宽度", [["narrow", "窄"], ["comfort", "舒适"], ["wide", "宽"]]);
      const writing = section(panel, "书写与保存");
      toggle(writing, "liveRendering", "实时渲染", "书写时在原位显示 Markdown 格式。");
      toggle(writing, "showActiveSyntax", "显示当前语法", "编辑当前段落时保留 Markdown 标记。");
      toggle(writing, "autoSave", "自动保存", "停止输入两秒后保存已有路径的文档。");
      const personalize = section(panel, "个性化");
      const appearance = destination(personalize, "外观", "Ember 主题 · 浅色 / 深色 / 跟随系统", "", 1, true);
      const refreshAppearance = () => { appearance.querySelector("span:last-of-type")!.textContent = `Ember · ${{ light: "浅色", dark: "深色", system: "跟随系统" }[current.theme]}`; };
      bindings.push(refreshAppearance); refreshAppearance();
      destination(personalize, "快捷键", "查看并自定义书写快捷键。", "自定义", 2);
    } else if (id === "appearance") {
      panel.classList.add("settings-appearance");
      panel.append(node("p", "settings-intro", "让每一份 Markdown，都有适合阅读的样子。"));
      segmented(panel, "theme", "外观模式", [["light", "浅色"], ["dark", "深色"], ["system", "跟随系统"]]);
      const previews = node("div", "settings-theme-previews");
      for (const [mode, label] of [["light", "Ember Light"], ["dark", "Ember Dark"]] as const) {
        const preview = node("article", `settings-theme-preview settings-theme-${mode}`);
        preview.append(node("span", "settings-theme-name", label), node("h4", "settings-preview-title", "在宁静中，\n看见清晰。"), node("p", "settings-preview-text", "写下一点，再读一遍。\n让下一个想法，慢慢成形。"), node("blockquote", "", "为重要的事，留一点空间。"), node("span", "settings-preview-link", "循着一个小小的念头 ↗"));
        previews.append(preview);
      }
      panel.append(previews);
      const hint = node("p", "settings-theme-hint");
      hint.append(createElement(Monitor, { width: 16, height: 16, "aria-hidden": "true" }), node("span", "", "跟随系统时，自动使用对应的浅色或深色主题。"));
      panel.append(hint, node("p", "settings-theme-scope", "应用于所有打开的文档"));
    } else if (id === "shortcuts") {
      panel.append(node("p", "settings-intro", options.shortcuts ? "当前快捷键绑定。" : "默认快捷键预设。"));
      const mod = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl+";
      const presets = options.shortcuts ?? [
        { label: "新建文档", keys: `${mod}N` }, { label: "打开文件", keys: `${mod}O` },
        { label: "保存", keys: `${mod}S` }, { label: "另存为", keys: `${mod}Shift+S` },
        { label: "加粗", keys: `${mod}B` }, { label: "斜体", keys: `${mod}I` },
        { label: "撤销", keys: `${mod}Z` }, { label: "重做", keys: `${mod}Shift+Z` },
        { label: "缩进", keys: "Tab" }, { label: "返回书写", keys: "Escape" },
      ];
      const list = node("dl", "settings-shortcuts");
      for (const preset of presets) {
        const entry = node("div", "settings-shortcut");
        const value = node("dd"); value.append(node("kbd", "", preset.keys));
        entry.append(node("dt", "", preset.label), value); list.append(entry);
      }
      panel.append(list);
      const customize = node("button", "settings-customize-shortcuts", "自定义快捷键…");
      customize.type = "button";
      customize.disabled = !options.onCustomizeShortcuts;
      customize.addEventListener("click", () => {
        // 等待关闭事件完成清理与焦点恢复，再打开下一层设置面板。
        afterClose = options.onCustomizeShortcuts;
        dialog.close();
      });
      panel.append(customize);
      if (!options.onCustomizeShortcuts) panel.append(node("p", "settings-description", "快捷键自定义入口尚未接入。"));
    } else {
      panel.append(node("p", "settings-intro", "管理书写习惯与本机数据。"));
      if (options.updates) {
        const updates = options.updates;
        const group = section(panel, "软件更新");
        const check = node("button", "settings-reset", "检查更新"); check.type = "button";
        const install = node("button", "settings-reset", "安装更新"); install.type = "button"; install.hidden = true;
        const version = node("p", "settings-description", "正在读取版本…");
        const message = node("p", "settings-description"); message.setAttribute("role", "status");
        const notes = node("p", "settings-description"); notes.style.whiteSpace = "pre-wrap";
        group.append(version, check, install, message, notes);
        const render = (value: Awaited<ReturnType<typeof updates.status>>) => {
          version.textContent = `当前版本：${value.version || "开发版"}`;
          check.disabled = !value.enabled || value.installed;
          install.hidden = !value.available || value.installed;
          install.disabled = false;
          notes.textContent = value.notes;
          message.textContent = value.installed ? "更新已安装，下次启动生效。请先保存文档后再关闭应用。"
            : !value.enabled ? "开发版或安装目录不可写时无法自动更新。"
            : value.available ? `新版本 ${value.available} 可供安装。` : "";
        };
        check.disabled = true;
        void updates.status().then(render).catch(() => { message.textContent = "无法读取更新状态，请重新打开设置。"; });
        check.addEventListener("click", async () => {
          check.disabled = true; install.disabled = true; message.textContent = "正在检查更新…";
          try {
            const value = await updates.check(); render(value);
            if (!value.available) message.textContent = "已是最新版本。";
          } catch (error) { message.textContent = error instanceof Error ? error.message : String(error); check.disabled = false; install.disabled = false; }
        });
        install.addEventListener("click", async () => {
          check.disabled = true; install.disabled = true; message.textContent = "正在下载并安装更新，请稍候…";
          try { render(await updates.install()); }
          catch (error) { message.textContent = error instanceof Error ? error.message : String(error); check.disabled = false; install.disabled = false; }
        });
      }
      const habits = section(panel, "书写习惯");
      toggle(habits, "focusMode", "专注模式", "淡化当前段落之外的文字。");
      toggle(habits, "typewriter", "打字机模式", "让光标所在行保持在视窗中央。");
      const notice = node("p", "settings-description", "恢复默认设置只影响偏好设置，不清理文档。");
      panel.append(notice);
      for (const [label, description, action] of [
        ["清理恢复草稿", "删除用于意外退出后恢复的草稿。", options.onClearDrafts],
        ["清理历史记录", "清理应用保存的历史记录。", options.onClearHistory],
      ] as const) {
        const button = node("button", "settings-danger", label); button.type = "button";
        button.disabled = !action;
        row(panel, label, action ? description : "此功能尚未接入。", button);
        const confirmation = node("div", "settings-confirmation"); confirmation.hidden = true;
        const confirm = node("button", "settings-danger", "确认清理"); confirm.type = "button";
        const cancel = node("button", "settings-reset", "取消"); cancel.type = "button";
        confirmation.append(node("p", "settings-description", "清理后无法恢复，是否继续？"), confirm, cancel); panel.append(confirmation);
        button.addEventListener("click", () => { confirmation.hidden = false; confirm.focus(); });
        cancel.addEventListener("click", () => { confirmation.hidden = true; button.focus(); });
        confirm.addEventListener("click", async () => {
          if (!action) return;
          button.disabled = true; confirm.disabled = true; cancel.disabled = true;
          try { await action(); confirmation.hidden = true; status.textContent = `${label}完成`; }
          catch { status.textContent = "清理未完成，请稍后重试。"; }
          finally { button.disabled = false; confirm.disabled = false; cancel.disabled = false; if (dialog.open) button.focus(); }
        });
      }
    }
  }
  function activate(index: number, focus = false) {
    panels.forEach(({ tab, panel }, i) => {
      tab.setAttribute("aria-selected", String(i === index)); tab.tabIndex = i === index ? 0 : -1; panel.hidden = i !== index;
    });
    heading.textContent = panels[index]!.label;
    if (focus) panels[index]!.tab.focus();
  }
  panels.forEach(({ tab }, index) => {
    tab.addEventListener("click", () => activate(index));
    tab.addEventListener("keydown", event => {
      let next: number;
      if (event.key === "ArrowDown" || event.key === "ArrowRight") next = (index + 1) % panels.length;
      else if (event.key === "ArrowUp" || event.key === "ArrowLeft") next = (index + panels.length - 1) % panels.length;
      else if (event.key === "Home") next = 0;
      else if (event.key === "End") next = panels.length - 1;
      else return;
      event.preventDefault(); activate(next, true);
    });
  });
  reset.addEventListener("click", () => { current = { ...defaultSettings }; bindings.forEach(refresh => refresh()); publish(); });
  // 在捕获阶段截住编辑器外的文档快捷键，避免设置中触发新建、保存等操作。
  dialog.addEventListener("keydown", event => {
    if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); dialog.close(); }
    else if ((event.metaKey || event.ctrlKey) && ["s", "o", "n"].includes(event.key.toLowerCase())) { event.preventDefault(); event.stopPropagation(); }
  }, true);
  dialog.addEventListener("close", () => {
    dialog.remove(); if (activeDialog === dialog) activeDialog = null;
    const editor = document.querySelector<HTMLElement>('#editor .cm-content[contenteditable="true"]');
    if (editor) editor.focus(); else if (previousFocus?.isConnected) previousFocus.focus();
    afterClose?.();
  }, { once: true });
  activate(0);
  document.body.append(dialog);
  dialog.showModal(); panels[0]!.tab.focus();
}
