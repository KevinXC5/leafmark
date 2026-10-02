import { isMyGo } from "mygo-runtime";
import { createElement, ChevronDown, ChevronRight, Folder, FolderOpen, FileText, Search, Ellipsis, type IconNode } from "lucide";
import { Files, Workspace, type Document as NoteDocument, type Folder as WorkspaceFolder, type Node as FolderNode, type RecentDocument } from "../platform/mygo";
import "./file-browser.css";

export interface FileBrowserServices {
  Files: Pick<typeof Files, "openWorkspace" | "openRecent">;
  Workspace: Pick<typeof Workspace, "current" | "tree" | "openFolder" | "createFile" | "createFolder" | "rename" | "clearRecent" | "remember" | "showInFolder">;
}

export interface FileBrowserCallbacks {
  /** 可注入桥接实现进行独立测试；默认使用生成的 native API。 */
  services?: FileBrowserServices;
  /** 由主界面的 perform 包装，统一等待草稿并锁定编辑器；组件内部不再次调用此回调。 */
  onAction: (action: () => Promise<void>) => Promise<unknown>;
  /** 接收 Files 返回的文档（包含 id），在 onAction 的执行范围内调用。 */
  onOpen: (document: NoteDocument) => void | Promise<void>;
  /** 返回所有已打开标签的绝对路径；未保存文档的空路径会被忽略。 */
  getOpenPaths: () => readonly string[];
  onError?: (error: unknown) => void;
  native?: boolean;
}

export interface FileBrowser {
  element: HTMLElement;
  /** 首次恢复工作区；错误会显示在组件内并通知 onError。 */
  ready: Promise<void>;
  /** 可在保存、关闭标签或清空历史后调用；同样经过 onAction。 */
  refresh: () => Promise<void>;
  dispose: () => void;
}

/** 替换 container 的内部内容，不修改容器自身的 id、hidden 或样式。 */
export function mountFileBrowser(container: HTMLElement, callbacks: FileBrowserCallbacks): FileBrowser {
  const services = callbacks.services ?? { Files, Workspace };
  const fileAPI = services.Files;
  const workspaceAPI = services.Workspace;
  const native = callbacks.native ?? isMyGo();
  const root = el("div", "fb-browser");
  const toolbar = el("div", "fb-toolbar");
  const choose = button("打开文件夹…", () => run(async () => {
    if (await workspaceAPI.openFolder()) {
      expanded.clear();
      selectedFolder = "";
      await refreshInside();
    }
  }));
  const workspaceMore = button("", () => {
    if (workspace) showMenu({ name: workspace.name, path: "", directory: true }, workspaceMore);
  }, "工作区更多操作");
  workspaceMore.className = "fb-more";
  workspaceMore.setAttribute("aria-label", "工作区更多操作");
  workspaceMore.setAttribute("aria-haspopup", "menu");
  workspaceMore.append(icon(Ellipsis));
  toolbar.append(choose, workspaceMore);
  toolbar.addEventListener("contextmenu", event => {
    event.preventDefault();
    if (!busy && workspace) showMenu({ name: workspace.name, path: "", directory: true }, workspaceMore, event.clientX, event.clientY);
  });
  const searchBox = el("div", "fb-search-box");
  const search = el("input", "fb-search");
  search.type = "search";
  search.placeholder = "搜索 Markdown…";
  search.setAttribute("aria-label", "搜索工作区文件或文件夹");
  searchBox.append(icon(Search), search);
  const target = el("p", "fb-target");
  const status = el("p", "fb-status");
  status.setAttribute("role", "status");
  status.setAttribute("aria-live", "polite");
  status.hidden = true;
  const tree = el("nav", "fb-tree");
  tree.setAttribute("aria-label", "工作区文件");
  const recentHeader = el("div", "fb-recent-header");
  const recentTitle = el("h3", "fb-section-title");
  recentTitle.textContent = "近期文件";
  const clear = button("清空", () => run(async () => {
    await workspaceAPI.clearRecent();
    await refreshInside();
  }), "清空近期文件记录（不删除文件）");
  recentHeader.append(recentTitle, clear);
  const recentList = el("div", "fb-recent-list");
  root.append(searchBox, toolbar, target, status, tree, recentHeader, recentList);
  container.replaceChildren(root);

  let workspace: WorkspaceFolder | null = null;
  let nodes: FolderNode[] = [];
  let recent: RecentDocument[] = [];
  let selectedFolder = "";
  let disposed = false;
  let busy = false;
  let menu: HTMLElement | null = null;
  let dialog: HTMLDialogElement | null = null;
  const expanded = new Set<string>();
  const listeners = new AbortController();
  const signal = listeners.signal;

  function report(error: unknown) {
    if (disposed) return;
    const message = error instanceof Error ? error.message : String(error);
    status.textContent = message;
    status.hidden = false;
    const validation = dialog?.querySelector<HTMLElement>(".fb-validation");
    if (validation) validation.textContent = message;
    callbacks.onError?.(error);
  }

  async function run(action: () => Promise<void>): Promise<void> {
    if (disposed || busy) return;
    if (!native) {
      report(new Error("文件导航请在桌面应用中使用。"));
      return;
    }
    busy = true;
    status.hidden = true;
    updateControls();
    let executed = false;
    try {
      await callbacks.onAction(async () => {
        executed = true;
        if (!disposed) {
          // 主界面的 perform 可能吞掉错误，因此在执行范围内报告组件操作失败。
          try { await action(); } catch (error) { report(error); }
        }
      });
      if (!executed && !disposed) report(new Error("当前有其他操作正在处理，请稍后重试。"));
    } catch (error) { report(error); }
    finally {
      busy = false;
      if (!disposed) updateControls();
    }
  }

  async function refreshInside() {
    const state = await workspaceAPI.current();
    const nextNodes = state.workspace ? await workspaceAPI.tree() : [];
    if (disposed) return;
    if (workspace?.path !== state.workspace?.path) {
      expanded.clear();
      selectedFolder = "";
    }
    workspace = state.workspace;
    nodes = nextNodes;
    recent = state.recent ?? [];
    const folders = new Set<string>();
    const collect = (items: FolderNode[]) => {
      for (const item of items) if (item.directory) {
        folders.add(item.path);
        collect(item.children ?? []);
      }
    };
    collect(nodes);
    if (selectedFolder && !folders.has(selectedFolder)) selectedFolder = "";
    for (const path of expanded) if (!folders.has(path)) expanded.delete(path);
    render();
  }

  function refresh() { return run(refreshInside); }

  function updateControls() {
    choose.disabled = !native || busy;
    workspaceMore.disabled = !native || busy || !workspace;
    clear.disabled = !native || busy || !recent.length;
    search.disabled = !native || !workspace;
    root.setAttribute("aria-busy", String(busy));
    for (const action of root.querySelectorAll<HTMLButtonElement>(".fb-node-open, .fb-more")) action.disabled = busy;
  }

  function render() {
    closeMenu();
    // 工作区名称兼作切换入口，避免重复堆叠名称、打开按钮与根目录提示。
    const workspaceLabel = el("span", "fb-node-label");
    workspaceLabel.textContent = workspace?.name ?? "打开文件夹…";
    choose.replaceChildren(...(workspace ? [icon(ChevronDown), icon(FolderOpen), workspaceLabel] : [icon(FolderOpen), workspaceLabel]));
    choose.title = workspace ? `${workspace.path}\n点击切换工作区` : "选择 Markdown 文件夹";
    choose.setAttribute("aria-label", workspace ? `切换工作区：${workspace.name}` : "打开文件夹");
    searchBox.hidden = workspaceMore.hidden = !workspace;
    target.textContent = selectedFolder ? `新建位置：${selectedFolder}` : "";
    target.hidden = !selectedFolder;
    tree.replaceChildren();
    const query = search.value.trim().toLocaleLowerCase();
    const matches = (node: FolderNode): boolean => node.path.toLocaleLowerCase().includes(query) ||
      node.name.toLocaleLowerCase().includes(query) || (node.children ?? []).some(matches);
    const append = (items: FolderNode[], parent: HTMLElement, showAll = false) => {
      for (const node of items) {
        if (query && !showAll && !matches(node)) continue;
        const row = el("div", "fb-node-row");
        const open = button("", () => {
          if (node.directory) {
            selectedFolder = node.path;
            if (expanded.has(node.path)) expanded.delete(node.path); else expanded.add(node.path);
            render();
          } else void run(async () => {
            await callbacks.onOpen(await fileAPI.openWorkspace(node.path));
            await refreshInside();
          });
        });
        open.className = "fb-node-open";
        open.title = node.path;
        const label = el("span", "fb-node-label");
        label.textContent = node.name;
        if (node.directory) {
          const isExpanded = Boolean(query) || expanded.has(node.path);
          open.append(icon(isExpanded ? ChevronDown : ChevronRight), icon(isExpanded ? FolderOpen : Folder));
          open.setAttribute("aria-expanded", String(isExpanded));
          row.classList.toggle("fb-selected", selectedFolder === node.path);
        } else open.append(icon(FileText));
        open.append(label);
        const more = button("⋯", () => showMenu(node, more), `${node.name}的更多操作`);
        more.className = "fb-more";
        more.setAttribute("aria-haspopup", "menu");
        row.append(open, more);
        row.addEventListener("contextmenu", event => {
          event.preventDefault();
          if (!busy) showMenu(node, more, event.clientX, event.clientY);
        });
        parent.append(row);
        if (node.directory && (query || expanded.has(node.path))) {
          const children = el("div", "fb-children");
          const folderMatches = node.path.toLocaleLowerCase().includes(query) || node.name.toLocaleLowerCase().includes(query);
          append(node.children ?? [], children, showAll || Boolean(query && folderMatches));
          if (!children.childElementCount) children.append(empty("空文件夹"));
          parent.append(children);
        }
      }
    };
    if (native && workspace) {
      const rootButton = button("⌂ 工作区根目录", () => { selectedFolder = ""; render(); });
      rootButton.className = "fb-root-target";
      rootButton.setAttribute("aria-pressed", String(!selectedFolder));
      // 仅在选中子目录后提供返回根目录的入口。
      rootButton.hidden = !selectedFolder;
      tree.append(rootButton);
      const entries = el("div", "fb-entries");
      append(nodes, entries);
      if (!entries.childElementCount) entries.append(empty(query ? "没有匹配的文件或文件夹。" : "工作区内没有 Markdown 文件或文件夹，可在此新建。"));
      tree.append(entries);
    } else tree.append(empty(native ? "选择一个文件夹，浏览并管理 Markdown 文档。" : "文件导航仅在桌面应用中可用。"));
    recentList.replaceChildren();
    for (const entry of recent) {
      const row = el("div", "fb-node-row");
      const open = button(entry.name, () => run(async () => {
        await callbacks.onOpen(await fileAPI.openRecent(entry.path));
        await refreshInside();
      }));
      open.className = "fb-node-open";
      open.title = entry.path;
      const reveal = button("↗", () => run(() => workspaceAPI.showInFolder(entry.path)), `在文件管理器中显示 ${entry.name}`);
      reveal.className = "fb-more";
      row.append(open, reveal);
      recentList.append(row);
    }
    recentHeader.hidden = recentList.hidden = !recent.length;
    updateControls();
  }

  function closeMenu() { menu?.remove(); menu = null; }

  function showMenu(node: FolderNode, anchor: HTMLElement, x?: number, y?: number) {
    closeMenu();
    menu = el("div", "fb-menu");
    menu.setAttribute("role", "menu");
    const add = (label: string, action: () => unknown) => {
      const item = button(label, () => { closeMenu(); action(); });
      item.setAttribute("role", "menuitem");
      menu!.append(item);
    };
    if (node.directory) {
      add("在此新建文件…", () => editPath("file", node.path));
      add("在此新建文件夹…", () => editPath("folder", node.path));
    }
    if (!node.path) {
      add("刷新工作区和近期文件", () => refresh());
      add("切换工作区…", () => choose.click());
    } else add("重命名…", () => editPath("rename", "", node));
    add(node.directory ? "显示工作区根目录" : "在文件管理器中显示", () => run(async () => {
      if (!workspace) throw new Error("请先选择工作区。");
      // 后端仅允许显示根目录或近期授权文件；先登记工作区文件再传绝对路径。
      if (node.directory) await workspaceAPI.showInFolder(workspace.path);
      else {
        await workspaceAPI.remember(node.path);
        const separator = workspace.path.includes("\\") ? "\\" : "/";
        const absolute = workspace.path.replace(/[\\/]+$/, "") + separator + node.path.replace(/\//g, separator);
        await workspaceAPI.showInFolder(absolute);
        await refreshInside();
      }
    }));
    document.body.append(menu);
    const rect = anchor.getBoundingClientRect();
    menu.style.left = `${Math.max(8, Math.min(x ?? rect.left, window.innerWidth - menu.offsetWidth - 8))}px`;
    menu.style.top = `${Math.max(8, Math.min(y ?? rect.bottom, window.innerHeight - menu.offsetHeight - 8))}px`;
    menu.querySelector<HTMLButtonElement>("button")?.focus();
  }

  function guardRename(node: FolderNode) {
    if (!workspace) throw new Error("请先选择工作区。");
    // Windows 路径大小写不敏感；目录必须使用分隔符边界，避免误匹配同名前缀。
    const windows = /^[A-Za-z]:[\\/]/.test(workspace.path) || workspace.path.startsWith("\\\\");
    const normalize = (path: string) => {
      const value = path.replace(/\\/g, "/").replace(/\/+$/, "");
      return windows ? value.toLocaleLowerCase() : value;
    };
    const absolute = normalize(`${workspace.path.replace(/[\\/]+$/, "")}/${node.path}`);
    if (callbacks.getOpenPaths().some(path => {
      const open = normalize(path);
      return open === absolute || (node.directory && open.startsWith(`${absolute}/`));
    })) throw new Error(node.directory ? "此文件夹包含已打开的文档，请先关闭相关标签后再重命名。" : "此文档正在打开，请先关闭它的标签后再重命名。");
  }

  function editPath(kind: "file" | "folder" | "rename", parent: string, node?: FolderNode) {
    if (disposed || busy || !workspace || !native) return;
    if (kind === "rename" && node) {
      try { guardRename(node); } catch (error) { report(error); return; }
    }
    dialog?.close();
    const currentDialog = el("dialog", "fb-dialog");
    dialog = currentDialog;
    const form = el("form");
    const title = el("h2");
    title.textContent = kind === "rename" ? "重命名" : kind === "file" ? "新建 Markdown 文件" : "新建文件夹";
    const label = el("label", "fb-field");
    label.textContent = kind === "rename" ? "新名称" : "相对路径";
    const input = el("input");
    input.required = true;
    input.value = kind === "rename" ? node!.name : parent ? `${parent}/` : "";
    input.placeholder = kind === "file" ? "例如 Ideas/新想法.md" : "例如 Ideas/资料";
    label.append(input);
    const hint = el("p");
    hint.textContent = kind === "rename" ? "仅修改名称；已打开的文档及其所在文件夹需先关闭相关标签。" : kind === "file" ? "相对于工作区根目录。文件名须使用 .md 或 .markdown 扩展名。" : "相对于工作区根目录；上级文件夹须已存在。";
    const validation = el("p", "fb-validation");
    validation.setAttribute("role", "alert");
    const actions = el("div", "fb-dialog-actions");
    const cancel = button("取消", () => currentDialog.close());
    const submit = button(kind === "rename" ? "重命名" : "创建", () => undefined);
    submit.type = "submit";
    submit.className = "fb-primary";
    actions.append(cancel, submit);
    form.append(title, label, hint, validation, actions);
    currentDialog.append(form);
    const workspacePath = workspace.path;
    let createdFilePath: string | null = null;
    form.addEventListener("submit", event => {
      event.preventDefault();
      if (busy) return;
      const value = input.value.trim();
      const invalid = !value || value.startsWith("/") || value.includes("\\") || /[\u0000-\u001f]/.test(value) ||
        /^[A-Za-z]:/.test(value) || value.split("/").some(part => !part || part === "." || part === "..");
      if (invalid || (kind === "rename" && value.includes("/"))) {
        validation.textContent = kind === "rename" ? "请输入有效名称，不能包含路径分隔符。" : "请输入工作区内的有效相对路径，不能包含 .. 或绝对路径。";
        return;
      }
      if ((kind === "file" || (kind === "rename" && !node!.directory)) && !/\.(md|markdown)$/i.test(value)) {
        validation.textContent = "Markdown 文件名须以 .md 或 .markdown 结尾。";
        return;
      }
      validation.textContent = "";
      submit.disabled = true;
      void run(async () => {
        if (workspace?.path !== workspacePath) throw new Error("工作区已变化，请重新操作。");
        if (kind === "rename") {
          guardRename(node!);
          const prefix = node!.path.slice(0, node!.path.lastIndexOf("/") + 1);
          const next = `${prefix}${value}`;
          if (next !== node!.path) await workspaceAPI.rename(node!.path, next);
          if (selectedFolder === node!.path) selectedFolder = next;
        } else if (kind === "folder") {
          await workspaceAPI.createFolder(value);
          selectedFolder = value;
          expanded.add(value);
        } else {
          // 创建已成功但打开失败时，重试同一路径只打开，避免再次创建触发已存在错误。
          if (createdFilePath !== value) {
            await workspaceAPI.createFile(value);
            createdFilePath = value;
          }
          await callbacks.onOpen(await fileAPI.openWorkspace(value));
        }
        currentDialog.close();
        await refreshInside();
      }).finally(() => { submit.disabled = false; });
    });
    currentDialog.addEventListener("close", () => {
      currentDialog.remove();
      if (dialog === currentDialog) dialog = null;
    });
    document.body.append(currentDialog);
    currentDialog.showModal();
    input.focus();
    if (kind === "rename") input.select();
    else input.setSelectionRange(input.value.length, input.value.length);
  }

  search.addEventListener("input", render);
  document.addEventListener("pointerdown", event => {
    if (menu && !menu.contains(event.target as globalThis.Node)) closeMenu();
  }, { signal });
  document.addEventListener("keydown", event => {
    if (event.key === "Escape" && menu) { closeMenu(); event.preventDefault(); }
    if (menu && (event.key === "ArrowDown" || event.key === "ArrowUp")) {
      const items = Array.from(menu.querySelectorAll<HTMLButtonElement>("button"));
      const index = items.indexOf(document.activeElement as HTMLButtonElement);
      items[(index + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length]?.focus();
      event.preventDefault();
    }
  }, { signal });
  window.addEventListener("resize", closeMenu, { signal });
  document.addEventListener("scroll", closeMenu, { signal, capture: true });
  render();
  const ready = native ? refresh() : Promise.resolve();
  return {
    element: root, ready, refresh,
    dispose() {
      disposed = true;
      listeners.abort();
      closeMenu();
      dialog?.remove();
      dialog = null;
      root.remove();
    },
  };
}

function el<K extends keyof HTMLElementTagNameMap>(tag: K, className?: string): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag);
  if (className) element.className = className;
  return element;
}

function button(label: string, action: () => unknown, title?: string) {
  const element = el("button");
  element.type = "button";
  element.textContent = label;
  element.title = title ?? label;
  element.addEventListener("click", () => { action(); });
  return element;
}

function icon(node: IconNode) {
  return createElement(node, { width: 14, height: 14, "stroke-width": 1.5, "aria-hidden": "true" });
}

function empty(message: string) {
  const element = el("p", "fb-empty");
  element.textContent = message;
  return element;
}
