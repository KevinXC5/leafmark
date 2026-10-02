import { afterEach, expect, test } from "bun:test";
import { clearRecovery, MAX_RECOVERY_NOTE_BYTES, readRecovery, RECOVERY_STORAGE_KEY, removeRecovery, writeRecovery, type RecoveredNote } from "../src/session-recovery";

const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
afterEach(() => {
  if (original) Object.defineProperty(globalThis, "localStorage", original);
  else Reflect.deleteProperty(globalThis, "localStorage");
});
const note = (id = "one", content = "未保存正文"): RecoveredNote => ({ id, name: "未命名.md", path: "", content, dirty: true });
function storage() {
  const data = new Map<string, string>([["unrelated", "保留"]]);
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => { data.set(key, value); },
    removeItem: (key: string) => { data.delete(key); },
  } });
  return data;
}

test("只持久化 dirty 草稿并丢弃额外字段", () => {
  const data = storage();
  expect(writeRecovery([note(), { ...note("saved"), dirty: false }, { ...note("two"), extra: "忽略" } as RecoveredNote])).toEqual({ ok: true });
  expect(readRecovery()).toEqual([note(), note("two")]);
  expect(data.get("unrelated")).toBe("保留");
});

test("读取损坏 JSON、非数组和无效记录", () => {
  const data = storage();
  for (const value of ["{broken", "null", "{}", "42"]) {
    data.set(RECOVERY_STORAGE_KEY, value); expect(readRecovery()).toEqual([]);
  }
  data.set(RECOVERY_STORAGE_KEY, JSON.stringify([note(), null, { ...note("bad"), dirty: "true" }, note(), { ...note("saved"), dirty: false }]));
  expect(readRecovery()).toEqual([note()]);
});

test("单份记录超限时保留原有快照，并按 UTF-8 计量", () => {
  const data = storage(); writeRecovery([note()]);
  const before = data.get(RECOVERY_STORAGE_KEY);
  expect(writeRecovery([note("large", "中".repeat(Math.ceil(MAX_RECOVERY_NOTE_BYTES / 3)))])).toMatchObject({ ok: false });
  expect(data.get(RECOVERY_STORAGE_KEY)).toBe(before);
  expect(writeRecovery([{ ...note(), name: "x".repeat(MAX_RECOVERY_NOTE_BYTES) }])).toMatchObject({ ok: false });
});

test("总容量超过 5 MB 时整体拒绝，不截断记录", () => {
  const data = storage(); writeRecovery([note()]); const before = data.get(RECOVERY_STORAGE_KEY);
  expect(writeRecovery(Array.from({ length: 6 }, (_, i) => note(String(i), "x".repeat(900_000))))).toMatchObject({ ok: false });
  expect(data.get(RECOVERY_STORAGE_KEY)).toBe(before);
});

test("无效字段或重复标识写入失败", () => {
  storage();
  expect(writeRecovery([{ ...note(), path: 123 } as unknown as RecoveredNote])).toMatchObject({ ok: false });
  expect(writeRecovery([note(), note()])).toMatchObject({ ok: false });
});

test("移除指定草稿和清理只操作指定 key", () => {
  const data = storage(); writeRecovery([note(), note("two")]);
  expect(removeRecovery("one")).toEqual({ ok: true }); expect(readRecovery()).toEqual([note("two")]);
  clearRecovery(); expect(data.has(RECOVERY_STORAGE_KEY)).toBe(false); expect(data.get("unrelated")).toBe("保留");
});

test("移除遇到损坏数据时不覆盖原始记录", () => {
  const data = storage(); data.set(RECOVERY_STORAGE_KEY, "{broken");
  expect(removeRecovery("one")).toMatchObject({ ok: false }); expect(data.get(RECOVERY_STORAGE_KEY)).toBe("{broken");
});

test("quota 和存储访问错误明确报告，读取安全回退", () => {
  storage();
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: {
    getItem: () => JSON.stringify([note()]),
    setItem: () => { throw new Error("QuotaExceededError"); },
    removeItem: () => { throw new Error("SecurityError"); },
  } });
  expect(writeRecovery([note()])).toMatchObject({ ok: false, error: expect.any(String) });
  expect(removeRecovery("one")).toMatchObject({ ok: false }); expect(() => clearRecovery()).toThrow();
  Object.defineProperty(globalThis, "localStorage", { configurable: true, get() { throw new Error("SecurityError"); } });
  expect(readRecovery()).toEqual([]); expect(writeRecovery([note()])).toMatchObject({ ok: false });
});
