import { afterEach, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { JSDOM } from "jsdom";
import { exportHTML } from "../src/render-markdown";
import { createDocumentSession, safeRecoveryName } from "../src/document-session";
import { EditorState } from "@codemirror/state";
import { readRecovery, writeRecovery, RECOVERY_STORAGE_KEY, type RecoveredNote } from "../src/session-recovery";

// 导出 HTML 仍直接提取生产函数；会话行为直接调用模块，不复制实现。
const source = readFileSync(new URL("../src/main.ts", import.meta.url), "utf8");
function actualFunction(name: string) {
  const match = source.match(new RegExp(`^(?:async )?function ${name}\\([^]*?^}`, "m"));
  if (!match) throw Error(`找不到生产函数 ${name}，请同步测试入口`);
  return match[0];
}
const originalStorage = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
const originalWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
const windows: JSDOM[] = [];
afterEach(() => {
  for (const dom of windows.splice(0)) dom.window.close();
  if (originalStorage) Object.defineProperty(globalThis, "localStorage", originalStorage);
  else Reflect.deleteProperty(globalThis, "localStorage");
  if (originalWindow) Object.defineProperty(globalThis, "window", originalWindow);
  else Reflect.deleteProperty(globalThis, "window");
});
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
const doc = (id: string, content = "原文", dirty = false): RecoveredNote => ({
  id, name: `${id}.md`, path: `/tmp/${id}.md`, content, dirty,
});
const tick = async () => { for (let i = 0; i < 5; i++) await Promise.resolve(); };

function harness(old: RecoveredNote[] = []) {
  const dom = new JSDOM('<div id="save-status"></div>', { url: "https://leafmark.test" });
  windows.push(dom);
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: dom.window.localStorage });
  writeRecovery(old);
  const backend = new Map<string, RecoveredNote>([["a", doc("a")], ["b", doc("b")]]);
  const errors: unknown[] = [];
  const controls = { editable: true, text: "原文", onDraft: async (_id: string, _content: string) => {}, onList: async () => [...backend.values()].map(value => ({ ...value })) };
  const Files = {
    draft: async (id: string, content: string) => {
      await controls.onDraft(id, content);
      const entry = backend.get(id);
      if (!entry) throw Error("ErrStale");
      entry.content = content; entry.dirty = content !== "原文";
    },
    list: () => controls.onList(),
  };
  const timers = new Map<number, { callback: () => void; delay: number }>();
  let timerSerial = 0;
  let state = EditorState.create({ doc: controls.text });
  const settings = { autoSave: false };
  const saves: string[] = [];
  const session = createDocumentSession({
    native: true, initial: doc("a"), files: { ...Files, session: async () => ({ currentId: "", documents: [] }) },
    getState: () => state, setState: next => { state = next; controls.text = state.doc.toString(); },
    createState: content => EditorState.create({ doc: content }),
    setEditable: value => { controls.editable = value; },
    hasDialog: () => !!dom.window.document.querySelector("dialog[open]"),
    autoSave: () => settings.autoSave, save: async () => { saves.push(session.note.id); },
    onWorking: () => {}, onLock: () => {}, onUpdate: () => session.rememberSession(),
    onError: error => errors.push(error),
    setTimer: ((callback: () => void, delay: number) => {
      const id = ++timerSerial; timers.set(id, { callback, delay }); return id;
    }) as unknown as typeof setTimeout,
    clearTimer: ((id: number) => timers.delete(id)) as unknown as typeof clearTimeout,
  });
  session.rememberSession();
  const api = {
    ...session, settings, timers, saves,
    edit: (content: string) => {
      if (!controls.editable) return false;
      controls.text = content; state = state.update({ changes: { from: 0, to: state.doc.length, insert: content } }).state;
      session.queueDraft(); return true;
    },
    current: () => ({ ...session.note }), busy: () => session.busy,
    session: (id: string) => session.entries.get(id),
    addSession: (value: RecoveredNote) => session.entries.set(value.id, { note: { ...value }, saved: "原文", state }),
    loadNote: session.loadNote,
    state: () => state,
    setState: (next: EditorState) => { state = next; controls.text = state.doc.toString(); },
  };
  return { api, dom, controls, backend, errors };
}

function initializationHarness() {
  const { controls, errors } = harness();
  const pending = deferred<{ currentId: string; documents: (RecoveredNote & { savedContent: string })[] }>();
  let state = EditorState.create({ doc: "原文" });
  const session = createDocumentSession({
    native: true, initial: doc("browser"),
    files: { draft: async () => {}, list: async () => [], session: () => pending.promise },
    getState: () => state, setState: next => { state = next; },
    createState: content => EditorState.create({ doc: content }),
    setEditable: value => { controls.editable = value; },
    hasDialog: () => false, autoSave: () => false, save: async () => {},
    onWorking: () => {}, onLock: () => {}, onUpdate: () => {}, onError: error => errors.push(error),
  });
  const initialization = session.initialize().then(current => { if (current) session.loadNote(current); })
    .catch(error => errors.push(error)).finally(session.unlock);
  const api = {
    initialization, perform: session.perform, errors, sessions: session.entries,
    busy: () => session.busy, editable: () => controls.editable,
    current: () => session.note, edit: () => controls.editable,
  };
  return { pending, api };
}

test("真实 initialize 挂起 Session 时禁止编辑和新建，加载草稿与保存基线后解锁", async () => {
  const { api, pending } = initializationHarness();
  expect(api.busy()).toBe(true); expect(api.edit()).toBe(false);
  let newCalled = false;
  await api.perform(async () => { newCalled = true; });
  expect(newCalled).toBe(false);
  const a = { ...doc("a", "未保存正文", true), savedContent: "磁盘正文" };
  const b = { ...doc("b"), savedContent: "原文" };
  pending.resolve({ currentId: "a", documents: [a, b] }); await api.initialization;
  expect(api.current()).toEqual(a); expect(api.sessions.size).toBe(2);
  expect(api.sessions.get("a").saved).toBe("磁盘正文");
  expect(api.sessions.get("a").state.doc.toString()).toBe("未保存正文");
  expect(api.busy()).toBe(false); expect(api.edit()).toBe(true);
  await api.perform(async () => { newCalled = true; }); expect(newCalled).toBe(true);
});

test("真实 initialization Promise 在 Session 失败时报告错误并解锁", async () => {
  const { api, pending } = initializationHarness();
  pending.reject(Error("Session 失败")); await api.initialization;
  expect(api.errors[0].message).toBe("Session 失败");
  expect(api.busy()).toBe(false); expect(api.editable()).toBe(true);
});

test("真实 safeRecoveryName 为跨平台路径及非法名称生成 Markdown 文件名", () => {
  const normalize = safeRecoveryName;
  const cases: [string, string][] = [
    ["", "未命名.md"], [".md", "未命名.md"], [".MARKDOWN", "未命名.md"],
    ["/notes/想法.md", "想法.md"], ["C:\\笔记\\想法.markdown", "想法.markdown"],
    ["a:b.md", "a_b.md"], ["note.txt", "note.txt.md"], ["标题", "标题.md"],
    ["末尾.  ", "末尾.md"], ["a\u0000b\n.md", "a_b_.md"], ["   ", "未命名.md"],
    ["A.MD", "A.MD"], ['a<>:\"|?*.md', "a_______.md"],
  ];
  for (const [name, expected] of cases) {
    const result = normalize(name); expect(result).toBe(expected);
    expect(result).toMatch(/\.(?:md|markdown)$/i);
    expect(/[\\/\\\\:\x00-\x1f<>\"|?*]/.test(result)).toBe(false);
  }
});

test("外部变动标记阻止脏文档自动保存，取消原定时器，清除标记后恢复调度", async () => {
  const { api } = harness();
  api.settings.autoSave = true;
  api.edit("未保存正文"); await api.flush();
  const autoTimers = () => [...api.timers.values()].filter((timer: { delay: number }) => timer.delay === 2000);
  expect(autoTimers()).toHaveLength(1);
  api.externalStates.set("a", { changed: true });
  api.scheduleAutoSave(); expect(autoTimers()).toHaveLength(0);
  api.edit("外部变动后的正文"); await api.flush();
  expect(api.current().dirty).toBe(true); expect(autoTimers()).toHaveLength(0);
  api.externalStates.delete("a"); api.scheduleAutoSave();
  expect(autoTimers()).toHaveLength(1);
});

test("标签切换保留实际 EditorState 的选区和撤销历史，保存基线按标签隔离", async () => {
  const { api } = harness();
  const { history, undo } = await import("@codemirror/commands");
  api.setState(EditorState.create({ doc: "原文", extensions: [history()] }));
  api.edit("甲草稿");
  api.setState(api.state().update({ selection: { anchor: 2 } }).state);
  await api.flush();
  const aState = api.state();
  api.loadNote(doc("b", "乙原文"));
  api.edit("乙草稿"); await api.flush();
  api.loadNote({ ...doc("a"), name: "甲新名称.md" });
  expect(api.state()).toBe(aState);
  expect(api.state().selection.main.head).toBe(2);
  expect(api.current()).toMatchObject({ content: "甲草稿", name: "甲新名称.md", dirty: true });
  expect(undo({ state: api.state(), dispatch: transaction => api.setState(transaction.state) })).toBe(true);
  api.queueDraft(); await api.flush();
  expect(api.current()).toMatchObject({ content: "原文", dirty: false });
  api.loadNote(doc("b"));
  expect(api.current()).toMatchObject({ content: "乙草稿", dirty: true });
  api.acceptSave({ ...doc("b", "乙草稿"), name: "另存乙.md" }, "乙草稿");
  api.edit("乙原文"); await api.flush();
  expect(api.current()).toMatchObject({ name: "另存乙.md", dirty: true });
});

test("自动保存等待操作和对话框结束，旧标签回调不能保存新标签", async () => {
  const { api, dom } = harness();
  api.settings.autoSave = true;
  api.edit("甲草稿"); await api.flush();
  const timer = () => [...api.timers.values()].filter(value => value.delay === 2000).at(-1)!;
  const dialog = dom.window.document.createElement("dialog");
  dialog.setAttribute("open", ""); dom.window.document.body.append(dialog);
  const blocked = timer(); blocked.callback(); await tick();
  expect(api.saves).toEqual([]);
  expect(timer()).not.toBe(blocked);
  dialog.remove();
  const pending = deferred<void>();
  const working = api.perform(() => pending.promise);
  timer().callback(); await tick(); expect(api.saves).toEqual([]);
  pending.resolve(); await working;
  timer().callback(); await tick(); expect(api.saves).toEqual(["a"]);
  const stale = timer();
  api.loadNote(doc("b")); stale.callback(); await tick();
  expect(api.saves).toEqual(["a"]);
});

test("prepareClose 等待最新草稿入后端，等待期间禁止输入并即时持久化", async () => {
  const { api, controls, backend } = harness();
  const pending = deferred<void>(); controls.onDraft = () => pending.promise;
  api.edit("最后输入");
  let finished = false;
  const closing = api.lifecycle.prepareClose().then(() => { finished = true; });
  await tick();
  expect(finished).toBe(false); expect(api.busy()).toBe(true);
  expect(api.edit("不应接受")).toBe(false);
  expect(readRecovery().map(note => note.content)).toEqual(["最后输入"]);
  expect(backend.get("a")!.content).toBe("原文");
  pending.resolve(); await closing;
  expect(backend.get("a")!.content).toBe("最后输入");
});

test("perform 等待排队草稿再执行标签操作，重复操作被拦截", async () => {
  const { api, controls, backend } = harness();
  const pending = deferred<void>(); controls.onDraft = () => pending.promise;
  api.edit("甲的新正文");
  let called = 0;
  const switching = api.perform(async () => { called++; expect(backend.get("a")!.content).toBe("甲的新正文"); });
  await tick(); expect(called).toBe(0);
  await api.perform(async () => { called += 100; });
  pending.resolve(); await switching;
  expect(called).toBe(1); expect(api.busy()).toBe(false); expect(controls.editable).toBe(true);
});

test("连续输入按顺序同步，prepareClose 等待整条队列", async () => {
  const { api, controls, backend } = harness();
  const first = deferred<void>(), second = deferred<void>();
  const seen: string[] = [];
  controls.onDraft = (_id, content) => { seen.push(content); return content === "第一版" ? first.promise : second.promise; };
  api.edit("第一版"); api.edit("最终版");
  const closing = api.lifecycle.prepareClose(); await tick();
  expect(seen).toEqual(["第一版"]);
  first.resolve(); await tick(); expect(seen).toEqual(["第一版", "最终版"]);
  second.resolve(); await closing;
  expect(backend.get("a")!.content).toBe("最终版");
});

test("稍后草稿在编辑与关闭清理后保留，当前会话草稿被移除", async () => {
  const old = { ...doc("old", "尚未恢复", true), path: "" };
  const { api } = harness([old]);
  api.edit("当前草稿"); await api.flush(); api.persistRecovery();
  expect(readRecovery().map(note => note.id)).toEqual(["old", "a"]);
  api.lifecycle.discardRecovery(); expect(readRecovery()).toEqual([old]);
});

test("取消关闭等待 List 期间继续锁定，同步多标签保存元数据后解锁", async () => {
  const { api, controls } = harness();
  api.edit("甲草稿"); await api.flush();
  api.addSession(doc("b", "乙草稿", true));
  await api.lifecycle.prepareClose();
  const pending = deferred<RecoveredNote[]>(); controls.onList = () => pending.promise;
  const resuming = api.lifecycle.resume(); await tick();
  expect(api.busy()).toBe(true); expect(api.edit("抢跑输入")).toBe(false);
  pending.resolve([
    { ...doc("a", "甲草稿"), path: "/tmp/另存甲.md", name: "另存甲.md" },
    doc("b", "乙草稿", true),
  ]);
  await resuming;
  expect(api.current()).toMatchObject({ name: "另存甲.md", dirty: false, content: "甲草稿" });
  expect(api.session("a").saved).toBe("甲草稿");
  expect(api.session("b").note.dirty).toBe(true);
  expect(readRecovery().map(note => note.id)).toEqual(["b"]);
  expect(controls.editable).toBe(true); expect(api.busy()).toBe(false);
  expect(api.edit("取消后新输入")).toBe(true); await api.flush();
  expect(api.current().dirty).toBe(true);
});

test("prepareClose 草稿失败时解锁，下一次输入可以重建成功队列", async () => {
  const { api, controls } = harness();
  controls.onDraft = async () => { throw Error("草稿同步失败"); };
  api.edit("失败正文");
  await expect(api.lifecycle.prepareClose()).rejects.toThrow("草稿同步失败");
  expect(api.busy()).toBe(false); expect(controls.editable).toBe(true);
  controls.onDraft = async () => {};
  expect(api.edit("重试正文")).toBe(true); await api.flush();
  await api.lifecycle.prepareClose();
});

test("resume 列表失败仍解锁，并保留此前恢复副本", async () => {
  const { api, controls } = harness();
  api.edit("安全副本"); await api.lifecycle.prepareClose();
  controls.onList = async () => { throw Error("列表失败"); };
  await expect(api.lifecycle.resume()).rejects.toThrow("列表失败");
  expect(api.busy()).toBe(false); expect(controls.editable).toBe(true);
  expect(readRecovery()[0]!.content).toBe("安全副本");
});

test("已有操作或打开对话框时 prepareClose 拒绝，不干扰原操作锁", async () => {
  const { api, dom, controls } = harness();
  const pending = deferred<void>();
  const action = api.perform(() => pending.promise);
  await expect(api.lifecycle.prepareClose()).rejects.toThrow("请先结束当前操作");
  expect(api.busy()).toBe(true); expect(controls.editable).toBe(false);
  pending.resolve(); await action;
  const dialog = dom.window.document.createElement("dialog"); dialog.setAttribute("open", ""); dom.window.document.body.append(dialog);
  await expect(api.lifecycle.prepareClose()).rejects.toThrow("请先结束当前操作");
  expect(api.busy()).toBe(false);
});

test("真实 makeExportHTML 保留特殊 $ 字符、异步增强内容并清理临时节点", async () => {
  const dom = new JSDOM('<div id="app">应用正文</div>', { url: "https://leafmark.test" });
  windows.push(dom);
  Object.defineProperty(globalThis, "window", { configurable: true, value: dom.window });
  // 代码块避免数学扩展把美元符号当作公式分隔符。
  const literal = "价格 $&、$`、$'、$$、$1、$<name> 保持原样";
  const content = "```text\n" + literal + "\n```";
  const pending = deferred<void>();
  const makeExport = new Function("env", `
    const { document, DOMParser, exportHTML, enrichReading, content } = env;
    const session = { note: { name: '特殊字符.md' } };
    const editor = { state: { doc: { toString: () => content } } };
    ${new Bun.Transpiler({ loader: "ts" }).transformSync(actualFunction("makeExportHTML"))}
    return makeExportHTML();
  `);
  let mounted = false;
  const exporting = makeExport({
    document: dom.window.document, DOMParser: dom.window.DOMParser, exportHTML, content,
    enrichReading: async (article: HTMLElement) => {
      mounted = dom.window.document.body.contains(article);
      await pending.promise;
      const figure = article.ownerDocument.createElement("figure");
      figure.textContent = "异步图形 $& $` $' $$"; article.append(figure);
    },
  });
  expect(mounted).toBe(true);
  expect(dom.window.document.querySelector("main")).not.toBeNull();
  pending.resolve();
  const result = new dom.window.DOMParser().parseFromString(await exporting, "text/html");
  expect(result.querySelector("main pre code")!.textContent).toBe(literal + "\n");
  expect(result.querySelector("figure")!.textContent).toBe("异步图形 $& $` $' $$");
  expect(result.querySelectorAll("body")).toHaveLength(1);
  expect(result.querySelectorAll("main")).toHaveLength(1);
  expect(result.querySelector("main")!.hasAttribute("style")).toBe(false);
  expect(dom.window.document.body.innerHTML).toBe('<div id="app">应用正文</div>');
});

test("真实 makeExportHTML 异步增强失败仍清理临时节点", async () => {
  const dom = new JSDOM('<div id="app">应用正文</div>'); windows.push(dom);
  Object.defineProperty(globalThis, "window", { configurable: true, value: dom.window });
  const makeExport = new Function("document", "DOMParser", "exportHTML", "enrichReading", `
    const session = { note: { name: '失败.md' } }, editor = { state: { doc: { toString: () => '正文' } } };
    ${new Bun.Transpiler({ loader: "ts" }).transformSync(actualFunction("makeExportHTML"))}
    return makeExportHTML();
  `);
  await expect(makeExport(dom.window.document, dom.window.DOMParser, exportHTML, async () => { throw Error("增强失败"); })).rejects.toThrow("增强失败");
  expect(dom.window.document.body.innerHTML).toBe('<div id="app">应用正文</div>');
});

test("关闭清理遇到存储失败保留旧快照并报告错误", () => {
  const { api, dom } = harness([doc("old", "旧恢复正文", true)]);
  const before = dom.window.localStorage.getItem(RECOVERY_STORAGE_KEY);
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: { setItem: () => { throw Error("QuotaExceededError"); } } });
  expect(() => api.lifecycle.discardRecovery()).toThrow("无法保存恢复草稿");
  expect(dom.window.localStorage.getItem(RECOVERY_STORAGE_KEY)).toBe(before);
});
