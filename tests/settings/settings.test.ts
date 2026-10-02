import { afterEach, describe, expect, test } from "bun:test";
import { AUTO_SAVE_DELAY, defaultSettings, loadSettings, saveSettings, SETTINGS_STORAGE_KEY, validateSettings } from "../../src/settings/settings";

const originalStorage = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
afterEach(() => {
  if (originalStorage) Object.defineProperty(globalThis, "localStorage", originalStorage);
  else Reflect.deleteProperty(globalThis, "localStorage");
});

function storage(value: string | null = null) {
  const data = new Map<string, string>();
  if (value !== null) data.set(SETTINGS_STORAGE_KEY, value);
  const mock = {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, item: string) => { data.set(key, item); },
  };
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: mock });
  return mock;
}

describe("设置校验与持久化", () => {
  test("空值、数组和非对象返回独立默认值", () => {
    for (const value of [null, undefined, [], "设置", 42, false]) expect(validateSettings(value)).toEqual(defaultSettings);
    const result = validateSettings(null);
    result.fontSize = 28;
    expect(defaultSettings.fontSize).toBe(18);
    expect(AUTO_SAVE_DELAY).toBe(2000);
  });

  test("逐字段保留有效数据，剔除越界值、字符串布尔值和未知字段", () => {
    expect(validateSettings({
      font: "mono", fontSize: 29, lineHeight: "1.5", readingWidth: "wide", theme: "sepia",
      liveRendering: false, showActiveSyntax: "false", autoSave: true, focusMode: true,
      typewriter: false, injected: "<script>",
    })).toEqual({ ...defaultSettings, font: "mono", readingWidth: "wide", liveRendering: false, autoSave: true, focusMode: true });
  });

  test("字号和行高接受边界，拒绝非有限数与越界数", () => {
    for (const fontSize of [12, 28]) expect(validateSettings({ fontSize }).fontSize).toBe(fontSize);
    for (const lineHeight of [1.3, 2.2, 1.55]) expect(validateSettings({ lineHeight }).lineHeight).toBe(lineHeight);
    for (const fontSize of [11, 29, NaN, Infinity]) expect(validateSettings({ fontSize }).fontSize).toBe(defaultSettings.fontSize);
    for (const lineHeight of [1.2, 2.3, NaN, -Infinity]) expect(validateSettings({ lineHeight }).lineHeight).toBe(defaultSettings.lineHeight);
  });

  test("接受全部预设枚举，拒绝任意字体与宽度", () => {
    for (const font of ["newsreader", "serif", "sans", "mono"]) expect(validateSettings({ font }).font).toBe(font);
    for (const readingWidth of ["narrow", "comfort", "wide"]) expect(validateSettings({ readingWidth }).readingWidth).toBe(readingWidth);
    for (const theme of ["light", "dark", "system"]) expect(validateSettings({ theme }).theme).toBe(theme);
    expect(validateSettings({ font: "url(evil)", readingWidth: "100%" })).toEqual(defaultSettings);
  });

  test("读取损坏 JSON 或被禁用的存储不会抛出异常", () => {
    storage("{ broken");
    expect(loadSettings()).toEqual(defaultSettings);
    Object.defineProperty(globalThis, "localStorage", { configurable: true, get() { throw new Error("SecurityError"); } });
    expect(loadSettings()).toEqual(defaultSettings);
    expect(() => saveSettings({ ...defaultSettings })).not.toThrow();
  });

  test("写入经校验的设置并完成往返读取", () => {
    const mock = storage();
    saveSettings({ ...defaultSettings, theme: "dark", fontSize: 24, autoSave: true });
    expect(loadSettings()).toEqual({ ...defaultSettings, theme: "dark", fontSize: 24, autoSave: true });
    expect(JSON.parse(mock.getItem(SETTINGS_STORAGE_KEY)!)).toEqual(loadSettings());
  });

  test("存储空间不足时写入不会抛出异常", () => {
    Object.defineProperty(globalThis, "localStorage", { configurable: true, value: {
      getItem: () => null, setItem: () => { throw new Error("QuotaExceededError"); },
    } });
    expect(() => saveSettings({ ...defaultSettings })).not.toThrow();
  });
});
