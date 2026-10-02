import { afterEach, expect, test } from "bun:test";
import { defaultShortcuts, loadShortcuts, saveShortcuts, SHORTCUTS_STORAGE_KEY, validateShortcut } from "../../src/settings/shortcuts";

const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
afterEach(() => {
  if (original) Object.defineProperty(globalThis, "localStorage", original);
  else Reflect.deleteProperty(globalThis, "localStorage");
});
function storage() {
  const data = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => { data.set(key, value); },
  } });
  return data;
}
test("全部默认快捷键有效", () => {
  for (const shortcut of Object.values(defaultShortcuts)) expect(validateShortcut(shortcut)).toEqual({ valid: true, shortcut });
});
test("规范化修饰键顺序、别名、字母和功能键", () => {
  expect(validateShortcut("shift-ctrl-K")).toEqual({ valid: true, shortcut: "Ctrl-Shift-k" });
  expect(validateShortcut("alt-f2")).toEqual({ valid: true, shortcut: "Alt-F2" });
  expect(validateShortcut("cmd-k")).toEqual({ valid: true, shortcut: "Meta-k" });
  expect(validateShortcut("Mod-,")).toEqual({ valid: true, shortcut: "Mod-," });
});
test("拒绝裸主键、仅 Shift、未知键、重复修饰键和系统常用", () => {
  for (const value of ["a", "Shift-a", "F2", "Ctrl", "Ctrl-Enter", "Hyper-a", "Ctrl-Ctrl-k", "Mod-Ctrl-k", "Alt-F4", "Ctrl-r", "Mod-q", "Mod-c", "Ctrl-Shift-i", "Mod-F12", "Mod-Shift-z"]) {
    expect(validateShortcut(value).valid).toBe(false);
  }
});
test("绑定写入和读取往返，恢复默认对象相互独立", () => {
  storage();
  expect(saveShortcuts({ ...defaultShortcuts, bold: "Mod-Shift-b" })).toBe(true);
  expect(loadShortcuts().bold).toBe("Mod-Shift-b");
  const first = loadShortcuts(); first.bold = "Alt-b"; expect(loadShortcuts().bold).toBe("Mod-Shift-b");
});
test("拒绝完全冲突与跨平台 Mod 冲突，不覆盖已保存设置", () => {
  const data = storage(); saveShortcuts({ ...defaultShortcuts }); const before = data.get(SHORTCUTS_STORAGE_KEY);
  expect(saveShortcuts({ ...defaultShortcuts, bold: "Mod-s" })).toBe(false);
  expect(saveShortcuts({ ...defaultShortcuts, bold: "Ctrl-s" })).toBe(false);
  expect(saveShortcuts({ ...defaultShortcuts, bold: "Meta-s" })).toBe(false);
  expect(data.get(SHORTCUTS_STORAGE_KEY)).toBe(before);
});
test("损坏、不完整或冲突的存储回退默认值", () => {
  const data = storage();
  for (const value of ["{broken", "null", "[]", "{}", JSON.stringify({ ...defaultShortcuts, bold: "Mod-s" })]) {
    data.set(SHORTCUTS_STORAGE_KEY, value); expect(loadShortcuts()).toEqual(defaultShortcuts);
  }
});
test("存储不可用时读取安全回退、写入明确失败", () => {
  Object.defineProperty(globalThis, "localStorage", { configurable: true, get() { throw new Error("SecurityError"); } });
  expect(loadShortcuts()).toEqual(defaultShortcuts); expect(saveShortcuts({ ...defaultShortcuts })).toBe(false);
});
