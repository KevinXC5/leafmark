import { afterEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { mountFileBrowser, type FileBrowser, type FileBrowserServices } from "../../src/file-browser/file-browser";
import type { Document as NoteDocument, Node as FolderNode, State } from "../../src/platform/mygo";

const globals = ["window", "document", "AbortController"] as const;
const originals = new Map(globals.map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)]));
const active: { browser: FileBrowser; dom: JSDOM }[] = [];
afterEach(() => {
  for (const { browser, dom } of active.splice(0)) { browser.dispose(); dom.window.close(); }
  for (const key of globals) {
    const original = originals.get(key);
    if (original) Object.defineProperty(globalThis, key, original);
    else Reflect.deleteProperty(globalThis, key);
  }
});

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(yes => { resolve = yes; });
  return { promise, resolve };
}

function harness(options: { gate?: Promise<void>; root?: string; skip?: boolean } = {}) {
  const dom = new JSDOM('<section id="documents" hidden><p>旧内容</p></section>', { url: "https://leafmark.test" });
  for (const key of globals) Object.defineProperty(globalThis, key, { configurable: true, writable: true, value: dom.window[key] });
  // jsdom 不实现原生模态对话框，保留生产表单及关闭事件逻辑。
  dom.window.HTMLDialogElement.prototype.showModal = function () { this.open = true; };
  dom.window.HTMLDialogElement.prototype.close = function () { this.open = false; this.dispatchEvent(new dom.window.Event("close")); };
  const root = options.root ?? "/notes";
  const state: State = { workspace: { name: "笔记", path: root }, recent: [{ name: "<img src=x onerror=evil>", path: `${root}/a.md`, openedAt: "" }] };
  let nodes: FolderNode[] = [
    { name: "空目录", path: "空目录", directory: true, children: [] },
    { name: "资料", path: "资料", directory: true, children: [{ name: "a.md", path: "资料/a.md", directory: false }] },
    { name: "a.md", path: "a.md", directory: false },
  ];
  const calls: { name: string; args: string[] }[] = [];
  const opened: NoteDocument[] = [];
  const errors: unknown[] = [];
  const controls = { openPaths: [] as string[], inAction: false, gate: options.gate, skip: options.skip ?? false, renameError: null as Error | null, folderError: null as Error | null, openError: null as Error | null };
  const call = (name: string, ...args: string[]) => {
    expect(controls.inAction).toBe(true);
    calls.push({ name, args });
  };
  const doc = (path: string): NoteDocument => ({ id: "doc-1", name: "a.md", path, content: "正文", dirty: false });
  const services: FileBrowserServices = {
    Files: {
      openWorkspace: async path => { call("openWorkspace", path); if (controls.openError) throw controls.openError; return doc(`${root}/${path}`); },
      openRecent: async path => { call("openRecent", path); return doc(path); },
    },
    Workspace: {
      current: async () => { call("current"); return state; },
      tree: async () => { call("tree"); return nodes; },
      openFolder: async () => { call("openFolder"); return state.workspace; },
      createFile: async path => { call("createFile", path); nodes.push({ name: path, path, directory: false }); },
      createFolder: async path => { call("createFolder", path); if (controls.folderError) throw controls.folderError; nodes.push({ name: path, path, directory: true, children: [] }); },
      rename: async (oldPath, next) => {
        call("rename", oldPath, next);
        if (controls.renameError) throw controls.renameError;
        const node = nodes.find(node => node.path === oldPath)!;
        node.path = next; node.name = next.split("/").at(-1)!;
      },
      clearRecent: async () => { call("clearRecent"); state.recent = []; },
      remember: async path => { call("remember", path); },
      showInFolder: async path => { call("showInFolder", path); },
    },
  };
  const container = dom.window.document.getElementById("documents")!;
  const browser = mountFileBrowser(container, {
    native: true, services,
    onAction: async action => {
      if (controls.skip) return;
      if (controls.gate) await controls.gate;
      controls.inAction = true;
      try { await action(); } finally { controls.inAction = false; }
    },
    onOpen: document => { expect(controls.inAction).toBe(true); opened.push(document); },
    getOpenPaths: () => controls.openPaths,
    onError: error => errors.push(error),
  });
  active.push({ browser, dom });
  const click = (label: string, parent: ParentNode = container) => {
    // 新建入口收进目录菜单，仍验证原有的创建、校验与错误恢复行为。
    if (label === "新建文件" || label === "新建文件夹") {
      const selected = container.querySelector<HTMLButtonElement>(".fb-selected .fb-more") ?? container.querySelector<HTMLButtonElement>(".fb-toolbar .fb-more")!;
      selected.click();
      const item = Array.from(dom.window.document.querySelectorAll<HTMLButtonElement>('.fb-menu button')).find(button => button.textContent === `在此${label}…`)!;
      item.click();
      return;
    }
    const button = Array.from(parent.querySelectorAll("button")).find(button => button.textContent === label.replace(/^[▸▾·] /, ""));
    if (!button) throw new Error(`找不到按钮 ${label}`);
    button.click();
  };
  const menu = (name: string) => {
    const button = Array.from(container.querySelectorAll("button")).find(button => button.title === `${name}的更多操作`)!;
    button.click();
  };
  const submit = (value: string) => {
    (dom.window.document.querySelector("dialog input") as HTMLInputElement).value = value;
    dom.window.document.querySelector("dialog form")!.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  };
  const settle = async () => { for (let i = 0; i < 30; i++) await Promise.resolve(); };
  return { dom, browser, state, calls, opened, errors, controls, container, click, menu, submit, settle };
}

test("首次恢复仅调用 Workspace.current/tree，保留容器属性并安全显示名称", async () => {
  const h = harness(); await h.browser.ready;
  expect(h.calls.map(call => call.name)).toEqual(["current", "tree"]);
  expect(h.container.hidden).toBe(true);
  expect(h.container.id).toBe("documents");
  expect(h.container.textContent).not.toContain("旧内容");
  expect(h.container.querySelector("img")).toBeNull();
  expect(h.container.textContent).toContain("<img src=x onerror=evil>");
  h.click("▸ 空目录"); expect(h.container.textContent).toContain("空文件夹");
});

test("根目录箭头只折叠文件树，刷新保留折叠及子目录展开状态", async () => {
  const h = harness(); await h.browser.ready;
  h.click("资料");
  const collapse = h.container.querySelector<HTMLButtonElement>(".fb-root-toggle")!;
  const tree = h.container.querySelector<HTMLElement>(".fb-tree")!;
  collapse.click();
  expect(tree.hidden).toBe(true);
  expect(collapse.getAttribute("aria-expanded")).toBe("false");
  expect(h.calls.some(call => call.name === "openFolder")).toBe(false);
  await h.browser.refresh();
  expect(tree.hidden).toBe(true);
  collapse.click();
  expect(tree.hidden).toBe(false);
  expect(collapse.getAttribute("aria-expanded")).toBe("true");
  expect(tree.querySelector('.fb-node-open[aria-expanded="true"]')).not.toBeNull();
  expect(h.calls.some(call => call.name === "openFolder")).toBe(false);
});

test("文件夹单击展开，不插入新建位置或根目录区域", async () => {
  const h = harness(); await h.browser.ready;
  const choose = h.container.querySelector(".fb-workspace-choose") as HTMLButtonElement;
  expect(choose.textContent).toBe("笔记");
  expect(choose.title).toContain("/notes");
  h.click("▸ 资料");
  expect(h.container.querySelector('.fb-selected button[aria-expanded="true"]')).not.toBeNull();
  expect(h.container.querySelector(".fb-target")).toBeNull();
  expect(h.container.querySelector(".fb-root-target")).toBeNull();
  expect(h.container.textContent).not.toContain("新建位置");
  expect(h.container.querySelector(".fb-children")!.textContent).toContain("a.md");
  choose.click(); await h.settle();
  expect(h.calls.some(call => call.name === "openFolder")).toBe(true);
});

test("onAction 等待期间不访问 native；被主界面跳过的刷新可稍后重试", async () => {
  const gate = deferred(); const h = harness({ gate: gate.promise });
  expect(h.calls).toEqual([]);
  expect(h.container.querySelector('[aria-busy="true"]')).not.toBeNull();
  gate.resolve(); await h.browser.ready;
  expect(h.calls).toHaveLength(2);
  h.controls.skip = true; await h.browser.refresh(); expect(h.calls).toHaveLength(2);
  h.controls.skip = false; await h.browser.refresh(); expect(h.calls).toHaveLength(4);
});

test("工作区和近期文件打开均在 onAction 内传递带 id 的文档", async () => {
  const h = harness(); await h.browser.ready;
  h.click("· a.md"); await h.settle();
  h.click("<img src=x onerror=evil>"); await h.settle();
  expect(h.opened.map(doc => doc.id)).toEqual(["doc-1", "doc-1"]);
  expect(h.calls.filter(call => call.name.startsWith("open")).map(call => call.name)).toEqual(["openWorkspace", "openRecent"]);
});

test("新建文件夹保留空目录并支持在所选目录新建文件", async () => {
  const h = harness(); await h.browser.ready;
  h.click("新建文件夹"); h.submit("新目录"); await h.settle();
  expect(h.calls).toContainEqual({ name: "createFolder", args: ["新目录"] });
  expect(h.container.querySelector(".fb-selected")!.textContent).toContain("新目录");
  h.click("新建文件");
  expect((h.dom.window.document.querySelector("dialog input") as HTMLInputElement).value).toBe("新目录/");
  h.submit("新目录/新笔记.md"); await h.settle();
  expect(h.calls).toContainEqual({ name: "createFile", args: ["新目录/新笔记.md"] });
  expect(h.calls).toContainEqual({ name: "openWorkspace", args: ["新目录/新笔记.md"] });
  expect(h.opened[0]!.id).toBe("doc-1");
});

test("拒绝非法路径及非 Markdown 文件扩展名，不调用 native 写入", async () => {
  const h = harness(); await h.browser.ready; h.click("新建文件");
  for (const path of ["../越界.md", "/绝对.md", "C:\\越界.md", "资料//空.md", "资料/正文.txt"]) {
    h.submit(path); await h.settle();
    expect(h.dom.window.document.querySelector(".fb-validation")!.textContent).not.toBe("");
  }
  expect(h.calls.some(call => call.name === "createFile")).toBe(false);
});

test("禁止重命名已打开文件和包含已打开文档的文件夹", async () => {
  const h = harness(); await h.browser.ready;
  h.controls.openPaths = ["/notes/a.md"];
  h.menu("a.md"); h.click("重命名…", h.dom.window.document);
  expect(h.dom.window.document.querySelector("dialog")).toBeNull();
  expect(h.container.textContent).toContain("先关闭");
  h.controls.openPaths = ["/notes/资料/a.md"];
  h.menu("资料"); h.click("重命名…", h.dom.window.document);
  expect(h.dom.window.document.querySelector("dialog")).toBeNull();
  expect(h.calls.some(call => call.name === "rename")).toBe(false);
});

test("目录保护使用分隔符边界，Windows 路径忽略大小写", async () => {
  const h = harness({ root: "C:\\Notes" }); await h.browser.ready;
  h.controls.openPaths = ["c:\\notes\\资料\\a.md"];
  h.menu("资料"); h.click("重命名…", h.dom.window.document);
  expect(h.dom.window.document.querySelector("dialog")).toBeNull();
  h.controls.openPaths = ["C:\\Notes\\资料备份\\a.md"];
  h.menu("资料"); h.click("重命名…", h.dom.window.document);
  expect(h.dom.window.document.querySelector("dialog")).not.toBeNull();
  h.submit("参考资料"); await h.settle();
  expect(h.calls).toContainEqual({ name: "rename", args: ["资料", "参考资料"] });
});

test("重命名在 onAction 真正执行前再次检查路径", async () => {
  const h = harness(); await h.browser.ready;
  h.menu("a.md"); h.click("重命名…", h.dom.window.document);
  const gate = deferred(); h.controls.gate = gate.promise;
  h.submit("b.md"); h.controls.openPaths = ["/notes/a.md"];
  gate.resolve(); await h.settle();
  expect(h.calls.some(call => call.name === "rename")).toBe(false);
  expect(h.dom.window.document.querySelector(".fb-validation")!.textContent).toContain("先关闭");
});

test("重命名失败保留弹窗和输入，允许修正后重试", async () => {
  const h = harness(); await h.browser.ready;
  h.controls.renameError = new Error("名称已存在");
  h.menu("a.md"); h.click("重命名…", h.dom.window.document); h.submit("b.md"); await h.settle();
  expect(h.dom.window.document.querySelector(".fb-validation")!.textContent).toBe("名称已存在");
  h.controls.renameError = null; h.submit("c.md"); await h.settle();
  expect(h.dom.window.document.querySelector("dialog")).toBeNull();
  expect(h.container.textContent).toContain("c.md");
});

test("显示文件先登记相对路径并传绝对路径，文件夹显示工作区根目录", async () => {
  const h = harness(); await h.browser.ready;
  h.menu("a.md"); h.click("在文件管理器中显示", h.dom.window.document); await h.settle();
  expect(h.calls.slice(2, 4)).toEqual([{ name: "remember", args: ["a.md"] }, { name: "showInFolder", args: ["/notes/a.md"] }]);
  h.menu("资料"); h.click("显示工作区根目录", h.dom.window.document); await h.settle();
  expect(h.calls.at(-1)).toEqual({ name: "showInFolder", args: ["/notes"] });
});

test("搜索保留匹配目录路径，清空近期记录不删除文件，dispose 清理弹窗", async () => {
  const h = harness(); await h.browser.ready;
  const search = h.container.querySelector('input[type="search"]') as HTMLInputElement;
  search.value = "资料/a"; search.dispatchEvent(new h.dom.window.Event("input"));
  expect(h.container.textContent).toContain("资料"); expect(h.container.textContent).toContain("a.md");
  search.value = "没有匹配"; search.dispatchEvent(new h.dom.window.Event("input"));
  expect(h.container.textContent).toContain("没有匹配的文件或文件夹");
  h.click("清空"); await h.settle();
  expect(h.state.recent).toEqual([]);
  expect((h.container.querySelector(".fb-recent-header") as HTMLElement).hidden).toBe(true);
  expect((h.container.querySelector(".fb-recent-list") as HTMLElement).hidden).toBe(true);
  h.click("新建文件"); h.browser.dispose();
  expect(h.container.childElementCount).toBe(0);
  expect(h.dom.window.document.querySelector("dialog")).toBeNull();
  const count = h.calls.length; await h.browser.refresh(); expect(h.calls.length).toBe(count);
});


test("perform 跳过提交时保留输入并提示重试，提交按钮恢复", async () => {
  const h = harness(); await h.browser.ready; h.click("新建文件夹");
  h.controls.skip = true; h.submit("资料二"); await h.settle();
  expect(h.dom.window.document.querySelector(".fb-validation")!.textContent).toContain("稍后重试");
  expect((h.dom.window.document.querySelector('dialog button[type="submit"]') as HTMLButtonElement).disabled).toBe(false);
  expect(h.calls.some(call => call.name === "createFolder")).toBe(false);
  h.controls.skip = false; h.submit("资料二"); await h.settle();
  expect(h.calls).toContainEqual({ name: "createFolder", args: ["资料二"] });
  expect(h.dom.window.document.querySelector("dialog")).toBeNull();
});

test("新建文件夹失败后可修正重试", async () => {
  const h = harness(); await h.browser.ready; h.click("新建文件夹");
  h.controls.folderError = new Error("上级目录不存在"); h.submit("不存在/子目录"); await h.settle();
  expect(h.dom.window.document.querySelector(".fb-validation")!.textContent).toBe("上级目录不存在");
  h.controls.folderError = null; h.submit("新目录"); await h.settle();
  expect(h.container.querySelector(".fb-selected")!.textContent).toContain("新目录");
  expect(h.dom.window.document.querySelector("dialog")).toBeNull();
});

test("文件创建成功但打开失败时重试不重复创建", async () => {
  const h = harness(); await h.browser.ready; h.click("新建文件");
  h.controls.openError = new Error("暂时无法打开"); h.submit("新笔记.md"); await h.settle();
  expect(h.dom.window.document.querySelector(".fb-validation")!.textContent).toBe("暂时无法打开");
  h.controls.openError = null; h.submit("新笔记.md"); await h.settle();
  expect(h.calls.filter(call => call.name === "createFile")).toHaveLength(1);
  expect(h.calls.filter(call => call.name === "openWorkspace")).toHaveLength(2);
  expect(h.opened).toHaveLength(1);
});

test("工作区根路径以分隔符结尾时仍保护已打开文档", async () => {
  const h = harness({ root: "/" }); await h.browser.ready;
  h.controls.openPaths = ["/a.md"];
  h.menu("a.md"); h.click("重命名…", h.dom.window.document);
  expect(h.dom.window.document.querySelector("dialog")).toBeNull();
  expect(h.calls.some(call => call.name === "rename")).toBe(false);
});

test("刷新采用最新近期文件名称和路径元数据", async () => {
  const h = harness(); await h.browser.ready;
  h.state.recent = [{ name: "已重命名.md", path: "/notes/已重命名.md", openedAt: "2026-10-02T10:00:00Z" }];
  await h.browser.refresh();
  const recent = h.container.querySelector(".fb-recent-list")!;
  expect(recent.textContent).toContain("已重命名.md");
  expect(recent.textContent).not.toContain("<img");
  h.click("已重命名.md", recent); await h.settle();
  expect(h.calls).toContainEqual({ name: "openRecent", args: ["/notes/已重命名.md"] });
});

test("根目录折叠时搜索仍显示匹配结果", async () => {
  const h = harness(); await h.browser.ready;
  const tree = h.container.querySelector<HTMLElement>(".fb-tree")!;
  h.container.querySelector<HTMLButtonElement>(".fb-root-toggle")!.click();
  expect(tree.hidden).toBe(true);
  const search = h.container.querySelector<HTMLInputElement>(".fb-search")!;
  expect(search.placeholder).toBe("按文件名搜索…");
  search.value = "资料"; search.dispatchEvent(new h.dom.window.Event("input"));
  expect(tree.hidden).toBe(false);
  expect(tree.textContent).toContain("a.md");
  search.value = ""; search.dispatchEvent(new h.dom.window.Event("input"));
  expect(tree.hidden).toBe(true);
});

test("对话框内的操作失败只在对话框里提示", async () => {
  const h = harness(); await h.browser.ready;
  h.controls.folderError = new Error("上级文件夹不存在，请先创建上级文件夹");
  h.click("新建文件夹"); h.submit("甲/乙"); await h.settle();
  expect(h.dom.window.document.querySelector(".fb-validation")?.textContent).toBe("上级文件夹不存在，请先创建上级文件夹");
  expect(h.container.querySelector<HTMLElement>(".fb-status")!.hidden).toBe(true);
  expect(h.errors).toEqual([]);
});

test("菜单按 Escape 关闭后焦点回到触发按钮", async () => {
  const h = harness(); await h.browser.ready;
  const more = h.container.querySelector<HTMLButtonElement>(".fb-toolbar .fb-more")!;
  more.click();
  expect(h.dom.window.document.querySelector(".fb-menu")).not.toBeNull();
  h.dom.window.document.dispatchEvent(new h.dom.window.KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  expect(h.dom.window.document.querySelector(".fb-menu")).toBeNull();
  expect(h.dom.window.document.activeElement).toBe(more);
});
