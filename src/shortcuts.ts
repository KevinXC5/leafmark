import "./shortcuts.css";

export type ShortcutAction = "save" | "saveAs" | "open" | "new" | "close" | "find" | "bold" | "italic" | "link" | "reading" | "settings";
export type Shortcuts = Record<ShortcutAction, string>;
export const SHORTCUTS_STORAGE_KEY = "leafmark-shortcuts-v1";
export const defaultShortcuts: Readonly<Shortcuts> = Object.freeze({
  save: "Mod-s", saveAs: "Mod-Shift-s", open: "Mod-o", new: "Mod-n", close: "Mod-w",
  find: "Mod-f", bold: "Mod-b", italic: "Mod-i", link: "Mod-k", reading: "Mod-Shift-r", settings: "Mod-,",
});
const labels: Record<ShortcutAction, string> = {
  save: "保存", saveAs: "另存为", open: "打开文件", new: "新建文档", close: "关闭文档",
  find: "查找", bold: "加粗", italic: "斜体", link: "插入链接", reading: "阅读模式", settings: "设置",
};
const actions = Object.keys(defaultShortcuts) as ShortcutAction[];
export type ShortcutValidation = { valid: true; shortcut: string } | { valid: false; error: string };

/** 生成 CodeMirror 可直接使用的规范组合键；拒绝裸字母与系统保留组合。 */
export function validateShortcut(value: string): ShortcutValidation {
  if (typeof value !== "string") return { valid: false, error: "请输入有效组合键。" };
  const parts = value.trim().split("-");
  const key = parts.pop() ?? "";
  const aliases: Record<string, string> = { mod: "Mod", ctrl: "Ctrl", control: "Ctrl", meta: "Meta", cmd: "Meta", alt: "Alt", shift: "Shift" };
  const modifiers = parts.map(part => aliases[part.toLowerCase()]);
  if (modifiers.some(part => !part) || new Set(modifiers).size !== modifiers.length
    || (modifiers.includes("Mod") && (modifiers.includes("Ctrl") || modifiers.includes("Meta")))) {
    return { valid: false, error: "修饰键无效或重复，请使用 Mod、Ctrl、Meta、Alt、Shift。" };
  }
  if (!/^[a-z0-9,./;=\[\]\\]$/i.test(key) && !/^F(?:[1-9]|1[0-2])$/i.test(key)) return { valid: false, error: "主键须为单个字母、数字、标点或 F1–F12。" };
  const normalizedKey = /^f\d+$/i.test(key) ? key.toUpperCase() : key.toLowerCase();
  if (!modifiers.some(part => ["Mod", "Ctrl", "Meta", "Alt"].includes(part!))) return { valid: false, error: "请至少使用 Mod、Ctrl、Meta 或 Alt，避免影响正常输入。" };
  const primary = modifiers.some(part => ["Mod", "Ctrl", "Meta"].includes(part!));
  const shift = modifiers.includes("Shift"), alt = modifiers.includes("Alt");
  if ((primary && ["q", "h", "m", " ", "z", "x", "c", "v", "a"].includes(normalizedKey))
    || (primary && shift && ["t", "n", "j", "i", "c", "delete"].includes(normalizedKey))
    || (primary && !shift && ["r", "t", "l", "p", "j", "u"].includes(normalizedKey))
    || (alt && normalizedKey === "F4") || (primary && alt && normalizedKey === "F4")
    || ["F5", "F11", "F12"].includes(normalizedKey)) {
    return { valid: false, error: "此组合键属于系统、浏览器或基础编辑常用快捷键，请选择其他组合。" };
  }
  const ordered = ["Mod", "Ctrl", "Meta", "Alt", "Shift"].filter(part => modifiers.includes(part));
  return { valid: true, shortcut: [...ordered, normalizedKey].join("-") };
}

function signature(shortcut: string, mac: boolean): string {
  const parts = shortcut.replace(/Mod/g, mac ? "Meta" : "Ctrl").split("-");
  const key = parts.pop()!;
  return [...["Ctrl", "Meta", "Alt", "Shift"].filter(modifier => parts.includes(modifier)), key].join("-");
}
function conflict(bindings: Shortcuts, action: ShortcutAction, shortcut: string): ShortcutAction | undefined {
  // 同时检查两个平台，避免在另一平台因 Mod 与显式修饰键重合而冲突。
  return actions.find(other => other !== action && [false, true].some(mac => signature(bindings[other], mac) === signature(shortcut, mac)));
}
function checkedBindings(value: unknown): Shortcuts | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const bindings = { ...defaultShortcuts };
  for (const action of actions) {
    const raw = (value as Record<string, unknown>)[action];
    if (typeof raw !== "string") return null;
    const result = validateShortcut(raw);
    if (!result.valid) return null;
    bindings[action] = result.shortcut;
  }
  if (actions.some(action => conflict(bindings, action, bindings[action]))) return null;
  return bindings;
}
export function loadShortcuts(): Shortcuts {
  try { return checkedBindings(JSON.parse(localStorage.getItem(SHORTCUTS_STORAGE_KEY) ?? "null")) ?? { ...defaultShortcuts }; }
  catch { return { ...defaultShortcuts }; }
}
export function saveShortcuts(bindings: Shortcuts): boolean {
  const valid = checkedBindings(bindings);
  if (!valid) return false;
  try { localStorage.setItem(SHORTCUTS_STORAGE_KEY, JSON.stringify(valid)); return true; } catch { return false; }
}

let active: HTMLDialogElement | null = null;
function element<K extends keyof HTMLElementTagNameMap>(tag: K, text = "", className = ""): HTMLElementTagNameMap[K] {
  const result = document.createElement(tag); result.textContent = text; result.className = className; return result;
}
export function openShortcutSettings(onChange: (bindings: Shortcuts) => void): void {
  if (active?.open) { active.focus(); return; }
  let bindings = loadShortcuts();
  let candidate: string | null = null;
  const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const dialog = element("dialog", "", "leafmark-shortcuts"); active = dialog;
  const title = element("h2", "快捷键设置"); title.id = "leafmark-shortcuts-title";
  dialog.setAttribute("aria-labelledby", title.id);
  const intro = element("p", "选择动作，然后录制组合键。Mod 在 macOS 对应 ⌘，其他平台对应 Ctrl。");
  const label = element("label", "动作");
  const select = element("select"); select.id = "leafmark-shortcuts-action"; label.htmlFor = select.id;
  for (const action of actions) { const option = element("option", labels[action]); option.value = action; select.append(option); }
  const current = element("p", "", "shortcut-current");
  const record = element("button", "点击录制组合键", "shortcut-recorder"); record.type = "button";
  const hint = element("p", "至少包含一个 Mod、Ctrl、Meta 或 Alt；Escape 返回书写。");
  const status = element("p", "", "shortcut-status"); status.setAttribute("role", "status");
  const footer = element("div", "", "shortcut-actions");
  const restore = element("button", "恢复默认"); restore.type = "button";
  const apply = element("button", "应用绑定", "shortcut-primary"); apply.type = "button"; apply.disabled = true;
  const close = element("button", "返回书写"); close.type = "button";
  footer.append(restore, apply, close); dialog.append(title, intro, label, select, current, record, hint, status, footer);
  const refresh = () => { current.textContent = `当前绑定：${bindings[select.value as ShortcutAction]}`; candidate = null; apply.disabled = true; record.textContent = "点击录制组合键"; status.textContent = ""; };
  select.addEventListener("change", refresh);
  record.addEventListener("click", () => { record.textContent = "请按下组合键…"; record.focus(); });
  record.addEventListener("keydown", event => {
    if (event.key === "Escape") return;
    event.preventDefault(); event.stopPropagation();
    if (["Control", "Meta", "Alt", "Shift"].includes(event.key)) return;
    const mods: string[] = [];
    const mac = /Mac|iPhone|iPad/.test(navigator.platform);
    if (mac ? event.metaKey : event.ctrlKey) mods.push("Mod");
    if (mac && event.ctrlKey) mods.push("Ctrl");
    if (!mac && event.metaKey) mods.push("Meta");
    if (event.altKey) mods.push("Alt"); if (event.shiftKey) mods.push("Shift");
    // 字母从物理键位识别，避免 Option/Alt 或 Shift 把键值变成特殊字符。
    const key = /^Key[A-Z]$/.test(event.code) ? event.code.slice(3).toLowerCase() : /^Digit[0-9]$/.test(event.code) ? event.code.slice(5) : event.key;
    const result = validateShortcut([...mods, key].join("-"));
    candidate = null; apply.disabled = true;
    if (!result.valid) { status.textContent = result.error; return; }
    const other = conflict(bindings, select.value as ShortcutAction, result.shortcut);
    if (other) { status.textContent = `与“${labels[other]}”冲突，请录制其他组合键。`; return; }
    candidate = result.shortcut; record.textContent = result.shortcut; apply.disabled = false; status.textContent = "组合键可用，点击应用绑定。";
  });
  function persist(next: Shortcuts) {
    bindings = next;
    const saved = saveShortcuts(bindings);
    onChange({ ...bindings }); refresh();
    status.textContent = saved ? "绑定已生效并保存。" : "绑定已在本次会话生效，但无法保存到本地存储。";
  }
  apply.addEventListener("click", () => { if (candidate) persist({ ...bindings, [select.value]: candidate }); });
  restore.addEventListener("click", () => persist({ ...defaultShortcuts }));
  close.addEventListener("click", () => dialog.close());
  dialog.addEventListener("keydown", event => {
    if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); dialog.close(); }
    // 阻止主模块的文档快捷键；录制区仍需接收事件。
    else if ((event.metaKey || event.ctrlKey) && event.target !== record) { event.preventDefault(); event.stopPropagation(); }
  }, true);
  dialog.addEventListener("close", () => {
    dialog.remove(); if (active === dialog) active = null;
    const editor = document.querySelector<HTMLElement>('#editor .cm-content[contenteditable="true"]');
    if (editor) editor.focus(); else if (previous?.isConnected) previous.focus();
  }, { once: true });
  refresh(); document.body.append(dialog); dialog.showModal(); select.focus();
}
