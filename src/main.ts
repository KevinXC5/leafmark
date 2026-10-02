import "@fontsource/inter/400.css";
import "@fontsource/inter/500.css";
import "@fontsource/inter/600.css";
import "@fontsource/newsreader/400.css";
import "@fontsource/newsreader/400-italic.css";
import "@fontsource/newsreader/500.css";
import "@fontsource/newsreader/600.css";
import "@fontsource/geist-mono/400.css";
import "./style.css";
import { EditorState, Compartment } from "@codemirror/state";
import { EditorView, keymap, drawSelection, placeholder, highlightActiveLine } from "@codemirror/view";
import { history, historyKeymap, defaultKeymap, indentWithTab, undo, redo } from "@codemirror/commands";
import { markdown, markdownKeymap } from "@codemirror/lang-markdown";
import { GFM } from "@lezer/markdown";
import { search, searchKeymap, openSearchPanel } from "@codemirror/search";
import { applyFormat } from "./editor-actions";
import { renderMarkdown, exportHTML } from "./render-markdown";
import { loadSettings, saveSettings, openSettings, editorFonts, readingWidths, type Settings } from "./settings";
import { openTableEditor } from "./table-editor";
import { openLinkDialog, openImageDialog } from "./insert-dialogs";
import { readRecovery, writeRecovery, clearRecovery } from "./session-recovery";
import { loadShortcuts, openShortcutSettings, type ShortcutAction, type Shortcuts } from "./shortcuts";
import { createIcons, FileText, PanelLeft, Plus, Folder, FolderOpen, Save, Sun, Moon, Bold, Italic, Undo2, Redo2, Code, Quote, Heading1, Check, ChevronRight, Leaf, Strikethrough, Heading2, Heading3, List, ListOrdered, ListChecks, Link, Image, Table, SquareCode, Minus } from "lucide";
import { currentWindow, isMyGo, runtime } from "mygo-runtime";
import { renderDiagrams } from "./diagrams";
import { mountFileBrowser } from "./file-browser";
import { Files, Workspace, Assets, type Document as NoteDocument } from "./mygo";
import { liveMarkdown } from "./live-markdown";
import { liveTable } from "./live-table";
import { liveBlocks } from "./live-blocks";
import { sourceHighlighting } from "./source-highlighting";
import { analyzeDocument } from "./document-stats";
import { syntaxTree } from "@codemirror/language";
async function copyText(text: string) { if (native) await Workspace.copyText(text); else await navigator.clipboard.writeText(text); }
function editingExtensions() { return sourceMode ? sourceHighlighting() : liveExtensions(); }
function liveExtensions() { return [liveMarkdown({ showActiveSyntax: settings.showActiveSyntax, resolveImage: url => imageCache.get(`${note.id}:${url}`), onImageNeeded: loadLocalImage }), liveTable({ showActiveSyntax: settings.showActiveSyntax }), liveBlocks({ showActiveSyntax: settings.showActiveSyntax, copyText: copyText })]; }

const app = document.querySelector<HTMLDivElement>("#app")!;
const native = isMyGo();
const mac = native ? runtime().platform === "darwin" : /Mac/.test(navigator.platform);
document.documentElement.classList.toggle("mac", mac);
document.documentElement.classList.toggle("native", native);
const icons = { FileText, PanelLeft, Plus, Folder, FolderOpen, Save, Sun, Moon, Bold, Italic, Undo2, Redo2, Code, Quote, Heading1, Check, ChevronRight, Leaf, Strikethrough, Heading2, Heading3, List, ListOrdered, ListChecks, Link, Image, Table, SquareCode, Minus };
const refreshIcons = () => createIcons({ icons, attrs: { "stroke-width": 1.5 } });
app.innerHTML = `
  <aside class="sidebar" aria-label="文档导航">
    <div class="sidebar-top"><span class="brand"><i data-lucide="leaf"></i>叶笺</span><button class="icon-button" id="hide-sidebar" title="收起侧栏" aria-label="收起侧栏"><i data-lucide="panel-left"></i></button></div>
    <div class="sidebar-switch"><button id="documents-tab" aria-selected="false">文档</button><button id="outline-tab" class="selected" aria-selected="true">大纲</button></div>
    <div class="nav-content"><p class="section-label" id="nav-label">当前文档</p><nav id="outline" aria-label="文档大纲"></nav><section id="documents" hidden><input id="folder-filter" placeholder="筛选 Markdown…" aria-label="筛选 Markdown 文件"><div class="folder-tools"><button id="open-folder">打开文件夹…</button><button id="folder-new" title="新建工作区文件">＋</button><button id="folder-refresh" title="刷新">↻</button></div><div id="folder-tree"></div><p class="section-label">近期文档</p><div id="recent-files"></div></section></div>
    <div class="sidebar-bottom"><p class="section-label">本地文件</p><p class="file-location-row"><i data-lucide="folder"></i><span id="file-location" title="">尚未保存到磁盘</span></p><button class="open-button" id="open-file"><i data-lucide="folder-open"></i><span>打开文件…</span><kbd>${mac ? "⌘O" : "Ctrl+O"}</kbd></button><span class="sidebar-note">让想法，落在纸上。</span></div>
  </aside>
  <main class="workspace">
    <header class="titlebar"><button class="icon-button" id="show-sidebar" title="展开侧栏" aria-label="展开侧栏" hidden><i data-lucide="panel-left"></i></button><div class="document-tab"><i data-lucide="file-text"></i><span id="document-name">A small thought.md</span><span id="dirty-dot" hidden aria-label="未保存">●</span></div><button class="icon-button" id="new-file" title="新建文档" aria-label="新建文档"><i data-lucide="plus"></i></button><div class="titlebar-space"></div><button class="icon-button" id="more-actions" title="更多操作" aria-label="更多操作">⋯</button><button class="icon-button" id="save-file" title="保存文档" aria-label="保存文档"><i data-lucide="save"></i></button><button class="icon-button" id="theme-toggle" title="切换主题" aria-label="切换主题"><i data-lucide="moon"></i></button></header>
    <div class="app-actions" hidden><button id="file-menu-toggle">文件</button><button id="format-menu-toggle">格式</button><button id="find-toggle">查找</button><button id="reading-toggle">阅读</button><button id="settings-toggle">设置</button></div><div id="tab-list" class="tab-list" aria-label="打开的文档"></div><section id="editor" aria-label="Markdown 书写区"></section><article id="reading-view" class="markdown-body" hidden></article><section id="welcome-view" hidden><h1>给想法一点空间。</h1><p>从一页空白开始，或打开本地 Markdown 文件。</p><button id="welcome-new" class="primary">新建文档</button><button id="welcome-open">打开文件…</button></section>
    <footer class="statusbar"><div><span id="word-count">0 字</span><span id="read-time">1 分钟阅读</span></div><div><span id="save-status" role="status">就绪</span><span>UTF-8</span><span id="edit-mode">原位编辑</span><button id="mode-toggle" title="切换原位编辑与源码">源码</button></div></footer>
  </main>
  <div id="format-toolbar" class="format-toolbar" role="toolbar" aria-label="格式工具栏" hidden>
    <button data-format="bold" title="加粗" aria-label="加粗"><i data-lucide="bold"></i></button><button data-format="italic" title="斜体" aria-label="斜体"><i data-lucide="italic"></i></button><button data-format="strike" title="删除线" aria-label="删除线"><i data-lucide="strikethrough"></i></button><button data-format="code" title="行内代码" aria-label="行内代码"><i data-lucide="code"></i></button><button data-format="h1" title="一级标题" aria-label="一级标题"><i data-lucide="heading-1"></i></button><button data-format="h2" title="二级标题" aria-label="二级标题"><i data-lucide="heading-2"></i></button><button data-format="h3" title="三级标题" aria-label="三级标题"><i data-lucide="heading-3"></i></button><button data-format="bullet" title="无序列表" aria-label="无序列表"><i data-lucide="list"></i></button><button data-format="ordered" title="有序列表" aria-label="有序列表"><i data-lucide="list-ordered"></i></button><button data-format="task" title="任务列表" aria-label="任务列表"><i data-lucide="list-checks"></i></button><button data-format="quote" title="引用" aria-label="引用"><i data-lucide="quote"></i></button><button data-format="link" title="链接" aria-label="链接"><i data-lucide="link"></i></button><button data-format="image" title="图片" aria-label="图片"><i data-lucide="image"></i></button><button data-format="table" title="表格" aria-label="表格"><i data-lucide="table"></i></button><button data-format="codeblock" title="代码块" aria-label="代码块"><i data-lucide="square-code"></i></button><button data-format="hr" title="分隔线" aria-label="分隔线"><i data-lucide="minus"></i></button>
  </div>
  <dialog id="unsaved-dialog"><h2>先保存这段想法？</h2><p>当前文档有未保存的修改。</p><div class="dialog-actions"><button value="cancel">取消</button><button value="discard">不保存</button><button value="save" class="primary">保存</button></div></dialog>
  <div class="toast" id="toast" role="alert" hidden></div>
`;
refreshIcons();
const element = <T extends HTMLElement = HTMLElement>(id: string) => document.getElementById(id) as T;
const sample = "# Make room for a thought.\n\nA quiet afternoon. A simple document.\nLet your words keep pace with your mind.\n\n> Writing turns a passing thought into something clear.\n\n## Start with something small\n\nOpen a Markdown file and pick up where you left off. Headings, lists and quotes take shape as you type, right here in your document.\n\nMake space for **what matters**.\n\n### Three things for today\n\n- [ ] Capture a sudden idea\n- [ ] Write the first paragraph\n- [x] Leave a thought for tomorrow\n\n```javascript\nconst thought = 'Start here';\nwrite(thought);\n```\n\n## 写下一点想法\n\n中文、English 和 emoji 🌿 都可以在这里书写。选中一段文字，试试加粗或撤销。\n";
let note: NoteDocument = { id: "browser", name: "A small thought.md", path: "", content: sample, dirty: false };
let savedContent = sample;
let settings = loadSettings();
let shortcuts = loadShortcuts();
const shortcutKeys = new Compartment();
const imageCache = new Map<string, string>();
const imageRequests = new Set<string>();
async function loadLocalImage(url: string) {
  if (!native || !note.path) return;
  const id = note.id, key = `${id}:${url}`;
  if (imageRequests.has(key) || imageCache.has(key)) return;
  imageRequests.add(key);
  try {
    imageCache.set(key, await Assets.readImage(id, url));
    if (note.id === id && !sourceMode) editor.dispatch({ effects: live.reconfigure(liveExtensions()) });
  } catch { /* 资源未找到时保留 Markdown 路径供用户修正。 */ }
}
const sessions = new Map<string, { note: NoteDocument; saved: string; state: EditorState }>();
// 标签条每次重建都会清空子节点，先保留新建按钮的引用。
const newFileButton = element("new-file");
let autoSaveTimer: ReturnType<typeof setTimeout>;
let readingMode = false;
let recoveryTimer: ReturnType<typeof setTimeout>;
let recoveryWarning = "";
let recoveredDrafts = readRecovery();
function persistRecovery() {
  rememberSession();
  const result = writeRecovery([...recoveredDrafts, ...[...sessions.values()].map(entry => entry.note)]);
  if (!result.ok && result.error !== recoveryWarning) { recoveryWarning = result.error; showError(result.error); }
}

let busy = false;
let sourceMode = false;
const externalStates = new Map<string, "changed" | "missing">();
let draftQueue: Promise<unknown> = Promise.resolve();
let toastTimer: ReturnType<typeof setTimeout>;
const live = new Compartment();
const editable = new Compartment();
let lastWindowTitle = "";
let toolbarTimer: ReturnType<typeof setTimeout>;

function showError(error: unknown) {
  const message = error instanceof Error ? error.message : String(error);
  element("toast").textContent = message;
  element("toast").hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { element("toast").hidden = true; }, 8000);
  element("save-status").textContent = "操作未完成";
}

function scheduleAutoSave() {
  clearTimeout(autoSaveTimer);
  if (!settings.autoSave || !note.path || !note.dirty || externalStates.has(note.id)) return;
  const id = note.id;
  autoSaveTimer = setTimeout(() => {
    if (id !== note.id) return;
    if (busy || document.querySelector("dialog[open]")) { scheduleAutoSave(); return; }
    void perform(() => save(false));
  }, 2000);
}
function queueDraft() {
  note.content = editor.state.doc.toString();
  note.dirty = note.content !== savedContent;
  clearTimeout(recoveryTimer); recoveryTimer = setTimeout(persistRecovery, 500);
  scheduleAutoSave();
  if (native) {
    const { id, content } = note;
    // 串行发送草稿，防止较早的输入后到达后端并覆盖新输入。
    draftQueue = draftQueue.catch(() => {}).then(() => Files.draft(id, content));
    void draftQueue.catch(showError);
  }
}

async function reloadNote() {
  if (!native || !note.path || busy) return;
  await perform(async () => {
    if (note.dirty) {
      const dialog = document.createElement("dialog");
      const title = document.createElement("h2"); title.textContent = "重新读取磁盘内容？";
      const text = document.createElement("p"); text.textContent = "当前标签的未保存修改将被磁盘内容替换。可先取消并另存为草稿。";
      const actions = document.createElement("div"); actions.className = "dialog-actions";
      const cancel = document.createElement("button"); cancel.textContent = "取消"; cancel.onclick = () => dialog.close("cancel");
      const reload = document.createElement("button"); reload.textContent = "放弃修改并重新加载"; reload.className = "primary"; reload.onclick = () => dialog.close("reload");
      actions.append(cancel, reload); dialog.append(title, text, actions); document.body.append(dialog);
      dialog.showModal();
      const choice = await new Promise<string>(resolve => dialog.addEventListener("close", () => resolve(dialog.returnValue), { once: true }));
      dialog.remove(); if (choice !== "reload") return;
    }
    const doc = await Files.reload(note.id, true);
    externalStates.delete(note.id);
    for (const key of imageCache.keys()) { if (key.startsWith(`${note.id}:`)) imageCache.delete(key); }
    for (const key of imageRequests) { if (key.startsWith(`${note.id}:`)) imageRequests.delete(key); }
    sessions.delete(note.id);
    note = { id: "", name: "", path: "", content: "", dirty: false };
    loadNote(doc);
    persistRecovery();
  });
}
let externalCheckRunning = false;
async function checkExternal() {
  if (!native || !note.path || busy || externalCheckRunning || document.querySelector("dialog[open]")) return;
  const id = note.id;
  externalCheckRunning = true;
  try {
    const status = await Files.checkExternal(id);
    if (note.id !== id) return;
    if (status.missing || status.changed) {
      externalStates.set(id, status.missing ? "missing" : "changed"); clearTimeout(autoSaveTimer);
    } else externalStates.delete(id);
    updateDocumentUI();
  } catch (error) { showError(error); }
  finally { externalCheckRunning = false; }
}
window.addEventListener("focus", () => void checkExternal());

function runShortcut(action: ShortcutAction) {
  if (busy || document.querySelector("dialog[open]")) return;
  switch (action) {
    case "save": void perform(() => save(false)); break;
    case "saveAs": void perform(() => save(true)); break;
    case "open": void perform(open); break;
    case "new": void perform(newNote); break;
    case "close": if (note.id) void perform(() => closeNote(note.id)); break;
    case "find": if (readingMode) toggleReading(); openSearchPanel(editor); break;
    case "bold": applyFormat(editor, "bold"); break;
    case "italic": applyFormat(editor, "italic"); break;
    case "link": openLinkDialog(editor); break;
    case "reading": toggleReading(); break;
    case "settings": element("settings-toggle").click(); break;
  }
}
function shortcutBindings() {
  return Object.entries(shortcuts).map(([action, key]) => ({ key, run: () => { runShortcut(action as ShortcutAction); return true; } }));
}
function applyShortcuts(next: Shortcuts) {
  shortcuts = next;
  editor.dispatch({ effects: shortcutKeys.reconfigure(keymap.of(shortcutBindings())) });
}
function extensions() {
  return [
    markdown({ extensions: [GFM] }), history(), drawSelection(), highlightActiveLine(), EditorView.lineWrapping,
    EditorState.phrases.of({ "Find": "查找", "Replace": "替换为", "next": "下一个", "previous": "上一个", "all": "全部选中", "match case": "区分大小写", "regexp": "正则表达式", "by word": "全词匹配", "replace": "替换", "replace all": "全部替换", "close": "关闭", "No matches found": "没有匹配项" }),
    search({ top: true }),

    placeholder("写下一点想法…"), live.of(editingExtensions()),
    editable.of(EditorView.editable.of(true)),
    EditorView.contentAttributes.of({ "aria-label": "Markdown 编辑器", spellcheck: "false" }),
    shortcutKeys.of(keymap.of(shortcutBindings())),
    keymap.of([
      ...searchKeymap, ...historyKeymap, ...markdownKeymap, ...defaultKeymap, indentWithTab,
    ]),
    EditorView.updateListener.of(update => {
      if (update.docChanged) { queueDraft(); updateDocumentUI(); }
      else if (syntaxTree(update.startState) !== syntaxTree(update.state)) updateDocumentUI();
      if (update.selectionSet || update.focusChanged || update.docChanged || update.viewportChanged) updateToolbar();
      if (update.selectionSet && !update.docChanged) {
        const headings = [...element("outline").querySelectorAll<HTMLElement>("button")];
        let current: HTMLElement | undefined;
        for (const heading of headings) { heading.classList.remove("active"); if (Number(heading.dataset.position) <= editor.state.selection.main.head) current = heading; }
        current?.classList.add("active");
      }
      if (settings.typewriter && update.selectionSet && !busy) requestAnimationFrame(() => editor.dispatch({ effects: EditorView.scrollIntoView(editor.state.selection.main.head, { y: "center" }) }));
    }),
  ];
}

const editor = new EditorView({ parent: element("editor"), state: EditorState.create({ doc: sample, extensions: extensions() }) });

function updateToolbar() {
  clearTimeout(toolbarTimer);
  toolbarTimer = setTimeout(() => {
    const toolbar = element("format-toolbar");
    const selection = editor.state.selection.main;
    if (selection.empty || !editor.hasFocus || sourceMode || busy) { toolbar.hidden = true; return; }
    const coords = editor.coordsAtPos(selection.from);
    if (!coords) { toolbar.hidden = true; return; }
    const editorRect = element("editor").getBoundingClientRect();
    if (coords.top < editorRect.top || coords.bottom > editorRect.bottom) { toolbar.hidden = true; return; }
    toolbar.hidden = false;
    toolbar.style.left = `${Math.min(window.innerWidth - toolbar.offsetWidth - 20, Math.max(20, coords.left))}px`;
    toolbar.style.top = `${Math.max(editorRect.top + 8, coords.top - 50)}px`;
  });
}

function updateDocumentUI() {
  element("document-name").textContent = note.name;
  element("dirty-dot").hidden = !note.dirty;
  document.title = note.id ? `${note.dirty ? "● " : ""}${note.name} · Leafmark` : "Leafmark · 叶笺";
  element("save-file").toggleAttribute("disabled", !note.id);
  if (native && document.title !== lastWindowTitle) { lastWindowTitle = document.title; void currentWindow.setTitle(document.title).catch(() => {}); }
  const fileLocation = element("file-location");
  fileLocation.textContent = note.path ? note.path.split(/[\\/]/).slice(0, -1).join(mac ? "/" : "\\") : "尚未保存到磁盘";
  fileLocation.title = note.path;
  const stats = analyzeDocument(editor.state);
  element("word-count").textContent = `${stats.wordCount} 字`;
  element("read-time").textContent = `${stats.readMinutes} 分钟阅读`;
  if (!busy) element("save-status").textContent = externalStates.get(note.id) === "missing" ? "原文件已移动或删除，请另存为" : externalStates.has(note.id) ? "磁盘文件已修改；可从文件菜单重新加载" : note.dirty ? "未保存" : note.path ? "已保存" : "就绪";
  const outline = element("outline");
  outline.replaceChildren();
  for (const heading of stats.headings) {
    const button = document.createElement("button");
    button.className = "outline-item";
    button.dataset.position = String(heading.from);
    button.style.paddingLeft = `${8 + (heading.level - 1) * 12}px`;
    const level = document.createElement("span");
    level.className = "heading-level"; level.textContent = `H${heading.level}`;
    const label = document.createElement("span"); label.textContent = heading.title;
    button.append(level, label);
    button.addEventListener("click", () => { editor.dispatch({ selection: { anchor: heading.from }, effects: EditorView.scrollIntoView(heading.from, { y: "start", yMargin: 45 }) }); editor.focus(); });
    outline.append(button);
  }
  [...outline.querySelectorAll<HTMLElement>("button[data-position]")].filter(button => Number(button.dataset.position) <= editor.state.selection.main.head).at(-1)?.classList.add("active");
  if (!outline.children.length) { const empty = document.createElement("p"); empty.className = "empty-outline"; empty.textContent = "输入 # 标题，建立文档大纲。"; outline.append(empty); }
  // 文件夹列表由独立异步刷新维护，正文输入不会重建文件树。
  renderTabs();
  if (readingMode) updateReading();
}

function rememberSession() {
  if (note.id) sessions.set(note.id, { note: { ...note }, saved: savedContent, state: editor.state });
}
function loadNote(next: NoteDocument) {
  clearTimeout(autoSaveTimer);
  rememberSession();
  const existing = sessions.get(next.id);
  note = existing ? { ...existing.note, path: next.path, name: next.name } : next;
  savedContent = existing?.saved ?? next.content;
  // 每个标签保留自己的选区和撤销栈，切换不会丢失编辑历史。
  editor.setState(existing?.state ?? EditorState.create({ doc: next.content, extensions: extensions() }));
  editor.dispatch({ effects: [live.reconfigure(editingExtensions()), editable.reconfigure(EditorView.editable.of(!busy)), shortcutKeys.reconfigure(keymap.of(shortcutBindings()))] });
  element("welcome-view").hidden = true;
  element("editor").hidden = readingMode;
  element("reading-view").hidden = !readingMode;
  if (readingMode) updateReading();
  element("format-toolbar").hidden = true;
  updateDocumentUI();
  scheduleAutoSave();
}
async function switchNote(id: string) {
  if (id === note.id) return;
  await draftQueue;
  const cached = sessions.get(id);
  if (!cached) return;
  const next = native ? await Files.select(id) : cached.note;
  loadNote(next);
}
async function closeNote(id: string) {
  if (!id || !sessions.has(id)) return;
  if (id !== note.id) await switchNote(id);
  if (!(await mayReplace())) return;
  clearTimeout(autoSaveTimer);
  await draftQueue;
  if (native) await Files.close(id);
  sessions.delete(id);
  note = { id: "", name: "", path: "", content: "", dirty: false };
  const next = [...sessions.values()].at(-1);
  if (next) loadNote(native ? await Files.select(next.note.id) : next.note);
  else {
    editor.setState(EditorState.create({ extensions: extensions() }));
    element("editor").hidden = true; element("reading-view").hidden = true;
    element("welcome-view").hidden = false;
    updateDocumentUI();
  }
  persistRecovery();
}
function renderTabs() {
  rememberSession();
  const tabs = element("tab-list"); tabs.replaceChildren();
  for (const entry of sessions.values()) {
    const tab = document.createElement("div"); tab.className = `file-tab${entry.note.id === note.id ? " active" : ""}`;
    const button = document.createElement("button");
    button.className = "file-tab-label";
    button.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6M8 13h8M8 17h6"/></svg>';
    const label = document.createElement("span"); label.textContent = `${entry.note.dirty ? "● " : ""}${entry.note.name}`;
    button.append(label);
    button.title = entry.note.path || entry.note.name;
    button.onclick = () => void perform(() => switchNote(entry.note.id));
    const close = document.createElement("button"); close.textContent = "×"; close.className = "close-tab";
    close.setAttribute("aria-label", `关闭 ${entry.note.name}`); close.onclick = () => void perform(() => closeNote(entry.note.id));
    tab.append(button, close);
    tab.setAttribute("role", "tab"); tab.setAttribute("aria-selected", String(entry.note.id === note.id));
    tab.addEventListener("auxclick", event => { if (event.button === 1) { event.preventDefault(); void perform(() => closeNote(entry.note.id)); } });
    tabs.append(tab);
  }
  // 新建按钮紧跟在最后一个标签后，和原型的标签条一致；持有引用避免清空后被移除。
  tabs.append(newFileButton);
}

async function save(saveAs: boolean): Promise<boolean> {
  if (!note.id) return false;
  if (!native) {
    download(note.name || "未命名.md", editor.state.doc.toString(), "text/markdown;charset=utf-8");
    element("save-status").textContent = "已导出副本";
    return false;
  }
  await draftQueue;
  const content = editor.state.doc.toString();
  let saved: NoteDocument | null;
  try { saved = await Files.save(note.id, content, saveAs); }
  catch (error) { if (!saveAs && note.path && String(error).includes("文件已被其他程序修改")) { externalStates.set(note.id, "changed"); clearTimeout(autoSaveTimer); } throw error; }
  if (!saved) { element("save-status").textContent = note.dirty ? "未保存" : "已取消保存"; return false; }
  savedContent = content;
  externalStates.delete(note.id);
  note = { ...saved, content: editor.state.doc.toString(), dirty: editor.state.doc.toString() !== content };
  updateDocumentUI();
  persistRecovery();
  if (native) await refreshFolder();
  return true;
}

async function mayReplace() {
  if (!note.dirty) return true;
  const dialog = element<HTMLDialogElement>("unsaved-dialog");
  dialog.returnValue = "cancel";
  dialog.showModal();
  const choice = await new Promise<string>(resolve => { dialog.addEventListener("close", () => resolve(dialog.returnValue), { once: true }); });
  if (choice === "save") return await save(false);
  return choice === "discard";
}

async function open() {
  if (!native) { browserOpen(); return; }
  await draftQueue;
  const next = await Files.open();
  if (next) loadNote(next);
}
async function newNote() {
  await draftQueue;
  loadNote(native ? await Files.new() : { id: crypto.randomUUID(), name: "未命名.md", path: "", content: "", dirty: false });
  editor.focus();
}
async function perform(action: () => Promise<unknown>) {
  if (busy) return;
  busy = true;
  element("save-status").textContent = "处理中…";
  // 原生对话框开启时锁住正文，保持保存快照与屏幕上的文本一致。
  editor.dispatch({ effects: editable.reconfigure(EditorView.editable.of(false)) });
  updateToolbar();
  try { await draftQueue; await action(); }
  catch (error) { showError(error); }
  finally {
    busy = false;
    editor.dispatch({ effects: editable.reconfigure(EditorView.editable.of(true)) });
    updateDocumentUI();
  }
}

element("open-file").addEventListener("click", () => void perform(open));
element("save-file").addEventListener("click", () => void perform(() => save(false)));
element("new-file").addEventListener("click", () => void perform(newNote));
element("unsaved-dialog").addEventListener("cancel", () => { element<HTMLDialogElement>("unsaved-dialog").returnValue = "cancel"; });
element("unsaved-dialog").querySelectorAll<HTMLButtonElement>("button").forEach(button => button.addEventListener("click", () => element<HTMLDialogElement>("unsaved-dialog").close(button.value)));
function sidebar(show: boolean) {
  app.classList.toggle("sidebar-hidden", !show);
  element("show-sidebar").hidden = show;
  editor.requestMeasure();
}
element("hide-sidebar").addEventListener("click", () => sidebar(false));
element("show-sidebar").addEventListener("click", () => sidebar(true));
for (const tab of ["documents", "outline"]) {
  element(`${tab}-tab`).addEventListener("click", () => {
    for (const name of ["documents", "outline"]) {
      element(name).hidden = name !== tab;
      element(`${name}-tab`).classList.toggle("selected", name === tab);
      element(`${name}-tab`).setAttribute("aria-selected", String(name === tab));
    }
    element("nav-label").textContent = tab === "outline" ? "当前文档" : "打开的文档";
  });
}
function theme(dark: boolean) {
  document.documentElement.dataset.theme = dark ? "dark" : "light";
  element("theme-toggle").innerHTML = `<i data-lucide="${dark ? "sun" : "moon"}"></i>`;
  refreshIcons();
  editor.requestMeasure();
}
let themePreference: string | null = null;
try { themePreference = localStorage.getItem("leafmark-theme"); } catch {}
theme(themePreference ? themePreference === "dark" : matchMedia("(prefers-color-scheme: dark)").matches);
element("theme-toggle").addEventListener("click", () => {
  const dark = document.documentElement.dataset.theme !== "dark";
  settings.theme = dark ? "dark" : "light"; saveSettings(settings);
  theme(dark);
});
element("mode-toggle").addEventListener("click", () => {
  sourceMode = !sourceMode;
  settings.liveRendering = !sourceMode; saveSettings(settings);
  editor.dispatch({ effects: live.reconfigure(editingExtensions()) });
  element("edit-mode").textContent = sourceMode ? "源码编辑" : "原位编辑";
  element("mode-toggle").textContent = sourceMode ? "原位" : "源码";
  updateToolbar();
});
element("format-toolbar").addEventListener("mousedown", event => event.preventDefault());
element("format-toolbar").addEventListener("click", event => {
  const button = (event.target as Element).closest<HTMLButtonElement>("button");
  const action = button?.dataset.format;
  if (!action || busy) return;
  if (action === "link") openLinkDialog(editor);
  else if (action === "image") insertImage();
  else applyFormat(editor, action as Parameters<typeof applyFormat>[1]);
});
editor.scrollDOM.addEventListener("scroll", updateToolbar);
element("reading-view").addEventListener("click", event => {
  const link = (event.target as Element).closest<HTMLAnchorElement>('a[href^="#"]');
  if (!link) return;
  event.preventDefault();
  let id: string;
  try { id = decodeURIComponent(link.getAttribute("href")!.slice(1)); } catch { return; }
  [...element("reading-view").querySelectorAll<HTMLElement>("[id]")].find(node => node.id === id)?.scrollIntoView({ block: "start" });
});
window.addEventListener("resize", updateToolbar);
// 编辑器之外也响应文档快捷键，避免打开对话框后焦点移走导致保存失效。
document.addEventListener("keydown", event => {
  if (editor.hasFocus || document.querySelector("dialog[open]") || (event.target instanceof HTMLElement && event.target.matches("input,textarea,select"))) return;
  for (const [action, shortcut] of Object.entries(shortcuts)) {
    const parts = shortcut.split("-"); const key = parts.pop()!;
    const mod = parts.includes("Mod");
    const ctrl = parts.includes("Ctrl") || mod && !mac;
    const meta = parts.includes("Meta") || mod && mac;
    if (event.ctrlKey === ctrl && event.metaKey === meta && event.altKey === parts.includes("Alt") && event.shiftKey === parts.includes("Shift") && event.key.toLowerCase() === key.toLowerCase()) {
      event.preventDefault(); runShortcut(action as ShortcutAction); break;
    }
  }
});
window.addEventListener("beforeunload", event => { persistRecovery(); if (!native && [...sessions.values()].some(entry => entry.note.dirty)) { event.preventDefault(); } });
updateDocumentUI();
function safeRecoveryName(name: string) {
  const base = name.split(/[\\/]/).at(-1)?.replace(/[\x00-\x1f<>:\"|?*]/g, "_").replace(/[ .]+$/, "") || "未命名.md";
  if (/^\.(md|markdown)$/i.test(base)) return "未命名.md";
  return /\.(md|markdown)$/i.test(base) ? base : `${base}.md`;
}
async function initialize() {
  if (native) {
    busy = true;
    editor.dispatch({ effects: editable.reconfigure(EditorView.editable.of(false)) });
    sessions.clear(); note.id = "";
    const session = await Files.session();
    for (const doc of session.documents) {
      sessions.set(doc.id, { note: doc, saved: doc.savedContent, state: EditorState.create({ doc: doc.content, extensions: extensions() }) });
    }
    const current = session.documents.find(doc => doc.id === session.currentId);
    if (current) loadNote(current);
    else { element("editor").hidden = true; element("welcome-view").hidden = false; updateDocumentUI(); }
  }
  if (!recoveredDrafts.length || import.meta.env.MODE === "verification") return;
  const dialog = document.createElement("dialog");
  const title = document.createElement("h2"); title.textContent = "找到未保存的草稿";
  const text = document.createElement("p"); text.textContent = `上次留下 ${recoveredDrafts.length} 份未保存草稿，可以恢复为新标签，再另存为文件。`;
  const actions = document.createElement("div"); actions.className = "dialog-actions";
  const later = document.createElement("button"); later.textContent = "稍后"; later.onclick = () => dialog.close();
  const restore = document.createElement("button"); restore.textContent = "恢复草稿"; restore.className = "primary";
  restore.onclick = () => { dialog.close(); void perform(async () => {
    for (const recovered of [...recoveredDrafts]) {
      await draftQueue;
      const fresh = native ? await Files.newNamed(safeRecoveryName(recovered.name)) : { id: crypto.randomUUID(), name: recovered.name || "未命名.md", path: "", content: "", dirty: false };
      loadNote(fresh);
      editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: recovered.content } });
      note.name = native ? fresh.name : recovered.name; note.path = ""; updateDocumentUI();
      await draftQueue;
      recoveredDrafts = recoveredDrafts.filter(entry => entry.id !== recovered.id);
    }
    await draftQueue; persistRecovery();
  }); };
  actions.append(later, restore); dialog.append(title, text, actions); document.body.append(dialog);
  dialog.onclose = () => dialog.remove(); dialog.showModal();
}

const initialization = initialize().catch(showError).finally(() => { busy = false; editor.dispatch({ effects: editable.reconfigure(EditorView.editable.of(true)) }); });
const fileBrowser = mountFileBrowser(element("documents"), {
  native,
  onAction: async action => { await initialization; return perform(action); },
  onOpen: doc => loadNote(doc),
  getOpenPaths: () => [...sessions.values()].map(entry => entry.note.path).filter(Boolean),
  onError: showError,
});
async function refreshFolder() { setTimeout(() => { void fileBrowser.refresh().catch(showError); }, 0); }

let readingPromise: Promise<void> = Promise.resolve();
function updateReading() {
  const container = element("reading-view");
  container.innerHTML = renderMarkdown(editor.state.doc.toString());
  readingPromise = enrichReading(container);
  void readingPromise.catch(showError);
}
async function enrichReading(container: HTMLElement) {
  if (native && note.path) {
    const id = note.id;
    await Promise.all([...container.querySelectorAll<HTMLImageElement>("img")].map(async image => {
      const url = image.getAttribute("src") ?? "";
      if (/^(?:https?:|data:|\/\/)/i.test(url)) return;
      try { const data = await Assets.readImage(id, url); if (container.contains(image)) image.src = data; }
      catch { image.alt = `${image.alt || "图片"}（本地资源未找到）`; image.removeAttribute("src"); }
    }));
  }
  await renderDiagrams(container);
  for (const link of container.querySelectorAll<HTMLAnchorElement>("a[href]")) {
    const href = link.getAttribute("href")!;
    if (!/^(?:https?:|mailto:|tel:)/i.test(href)) continue;
    link.target = "_blank"; link.rel = "noopener noreferrer";
    if (native) link.onclick = event => { event.preventDefault(); void Workspace.openExternal(href).catch(showError); };
  }
}
async function makeExportHTML() {
  const html = exportHTML(note.name, editor.state.doc.toString(), document.documentElement.dataset.theme === "dark" ? "dark" : "light");
  const parsed = new DOMParser().parseFromString(html, "text/html");
  const article = parsed.querySelector<HTMLElement>("main") ?? parsed.body;
  article.classList.add("markdown-body"); document.body.append(article); article.style.cssText = "position:fixed;left:-20000px;top:0;width:800px;opacity:0;pointer-events:none";
  try { await enrichReading(article); return "<!doctype html>\n" + parsed.documentElement.outerHTML.replace(/<body[^>]*>[\s\S]*<\/body>/, () => `<body>${article.outerHTML.replace(/ style=\"[^\"]*\"/, "")}</body>`); }
  finally { article.remove(); }
}
function toggleReading() {
  if (!note.id) return;
  readingMode = !readingMode;
  element("editor").hidden = readingMode;
  element("reading-view").hidden = !readingMode;
  element("reading-toggle").textContent = readingMode ? "编辑" : "阅读";
  if (readingMode) updateReading(); else { editor.requestMeasure(); editor.focus(); }
  updateToolbar();
}
function download(name: string, content: string, type: string) {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const link = document.createElement("a"); link.href = url; link.download = name; link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function browserOpen() {
  const input = document.createElement("input"); input.type = "file"; input.accept = ".md,.markdown,.txt";
  input.onchange = () => { const file = input.files?.[0]; if (!file) return;
    void file.text().then(content => { if (file.size > 16 * 1024 * 1024) throw Error("文档最大支持 16 MB");
      loadNote({ id: crypto.randomUUID(), name: file.name, path: "", content: content.replace(/^﻿/, "").replace(/\r\n/g, "\n"), dirty: false });
    }).catch(showError);
  }; input.click();
}
function applySettings(next: Settings) {
  settings = next; saveSettings(next);
  const root = document.documentElement;
  root.style.setProperty("--book", editorFonts[next.font]);
  app.classList.toggle("focus-mode", next.focusMode);
  app.classList.toggle("typewriter-mode", next.typewriter);
  sidebar(!next.focusMode);
  root.style.setProperty("--editor-size", `${next.fontSize}px`);
  root.style.setProperty("--editor-line-height", String(next.lineHeight));
  root.style.setProperty("--reading-width", `${readingWidths[next.readingWidth]}px`);
  sourceMode = !next.liveRendering;
  editor.dispatch({ effects: live.reconfigure(editingExtensions()) });
  element("edit-mode").textContent = sourceMode ? "源码编辑" : "原位编辑";
  element("mode-toggle").textContent = sourceMode ? "原位" : "源码";
  theme(next.theme === "dark" || next.theme === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  editor.requestMeasure();
  scheduleAutoSave();
}
function showMenu(anchor: HTMLElement, entries: [string, () => void][]) {
  document.querySelector(".action-menu")?.remove();
  const menu = document.createElement("div"); menu.className = "action-menu"; menu.setAttribute("role", "menu");
  for (const [label, action] of entries) {
    const button = document.createElement("button"); button.textContent = label; button.setAttribute("role", "menuitem");
    button.onclick = () => { menu.remove(); action(); }; menu.append(button);
  }
  document.body.append(menu); const rect = (anchor.closest("[hidden]") ? element("more-actions") : anchor).getBoundingClientRect();
  menu.style.left = `${Math.min(rect.left, innerWidth - menu.offsetWidth - 10)}px`; menu.style.top = `${rect.bottom + 4}px`;
  const dismiss = (event: Event) => { if (!menu.contains(event.target as Node) && event.target !== anchor) { menu.remove(); document.removeEventListener("pointerdown", dismiss); } };
  setTimeout(() => document.addEventListener("pointerdown", dismiss), 0);
  menu.onkeydown = event => { if (event.key === "Escape") { menu.remove(); anchor.focus(); } };
  menu.querySelector<HTMLButtonElement>("button")?.focus();
}
element("file-menu-toggle").onclick = () => showMenu(element("file-menu-toggle"), [
  ["新建文档　⌘ / Ctrl+N", () => void perform(newNote)],
  ["打开文件…　⌘ / Ctrl+O", () => void perform(open)],
  ["保存　⌘ / Ctrl+S", () => void perform(() => save(false))],
  ["另存为…", () => void perform(() => save(true))],
  ["检查磁盘修改", () => void checkExternal()],
  ["从磁盘重新加载…", () => void reloadNote()],
  ["导出 Markdown 副本", () => { if (!note.id) return; if (native) void perform(() => Files.exportMarkdown(note.id, editor.state.doc.toString())); else download(note.name || "未命名.md", editor.state.doc.toString(), "text/markdown;charset=utf-8"); }],
  ["复制 Markdown", () => { const text = editor.state.doc.toString(); void (native ? Workspace.copyText(text) : navigator.clipboard.writeText(text)).catch(showError); }],
  ["导出 HTML", () => void perform(async () => { const html = await makeExportHTML(); if (native) await Workspace.exportHTML(note.name, html); else download((note.name || "未命名.md").replace(/\.(md|markdown)$/i, "") + ".html", html, "text/html;charset=utf-8"); })],
  ["打印 / 导出 PDF…", () => void perform(async () => { readingMode = true; element("editor").hidden = true; element("reading-view").hidden = false; updateReading(); await readingPromise; if (native) await Workspace.print(); else window.print(); })],
  ["关闭当前标签", () => void perform(() => closeNote(note.id))],
]);
function insertImage() {
  openImageDialog(editor, native ? { browse: async () => {
    const id = note.id; const result = await Assets.importImage(id);
    if (!result) return null;
    if (result.path) imageCache.set(`${id}:${result.path}`, result.dataURI);
    return { url: result.path || result.dataURI };
  } } : undefined);
}
element("format-menu-toggle").onclick = () => showMenu(element("format-menu-toggle"), [
  ...([ ["加粗", "bold"], ["斜体", "italic"], ["删除线", "strike"], ["行内代码", "code"], ["一级标题", "h1"], ["二级标题", "h2"], ["三级标题", "h3"], ["引用", "quote"], ["无序列表", "bullet"], ["有序列表", "ordered"], ["任务列表", "task"], ["代码块", "codeblock"], ["插入表格", "table"], ["分隔线", "hr"] ] as [string,string][]).map(([label, action]): [string,()=>void] => [label, () => { applyFormat(editor, action as Parameters<typeof applyFormat>[1]); editor.focus(); }]),
  ["链接…", () => openLinkDialog(editor)], ["图片…", insertImage],
  ["自定义快捷键…", () => openShortcutSettings(applyShortcuts)],
  ["编辑当前表格…", () => { if (!openTableEditor(editor)) showError("请将光标放在 Markdown 表格中。先插入一个表格也可以。"); }],
]);
element("find-toggle").onclick = () => { if (readingMode) toggleReading(); openSearchPanel(editor); };
element("reading-toggle").onclick = toggleReading;
element("settings-toggle").onclick = () => openSettings(settings, applySettings, {
  onCustomizeShortcuts: () => openShortcutSettings(applyShortcuts),
  shortcuts: Object.entries(shortcuts).map(([label, keys]) => ({ label: ({save:"保存",saveAs:"另存为",open:"打开文件",new:"新建文档",close:"关闭标签",find:"查找替换",bold:"加粗",italic:"斜体",link:"链接",reading:"阅读模式",settings:"设置"} as Record<string,string>)[label] ?? label, keys })),
  onClearDrafts: () => { clearRecovery(); recoveredDrafts = []; },
  onClearHistory: native ? async () => { await Workspace.clearRecent(); await refreshFolder(); } : undefined,
});
element("more-actions").onclick = () => showMenu(element("more-actions"), [
  ["保存　⌘ / Ctrl+S", () => element("save-file").click()],
  ["切换浅色 / 深色主题", () => element("theme-toggle").click()],
  ["文件操作与导出…", () => element("file-menu-toggle").click()],
  ["格式与插入…", () => element("format-menu-toggle").click()],
  ["查找与替换", () => element("find-toggle").click()],
  ["切换阅读模式", toggleReading],
  ["设置…", () => element("settings-toggle").click()],
]);
element("welcome-new").onclick = () => void perform(newNote);
element("welcome-open").onclick = () => void perform(open);
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => { if (settings.theme === "system") applySettings(settings); });
applySettings(settings);

Object.assign(window, { leafmarkLifecycle: {
  prepareClose: async () => {
    if (busy || document.querySelector("dialog[open]")) throw Error("请先结束当前操作");
    busy = true; clearTimeout(autoSaveTimer);
    editor.dispatch({ effects: editable.reconfigure(EditorView.editable.of(false)) });
    try { persistRecovery(); await draftQueue; }
    catch (error) {
      busy = false; editor.dispatch({ effects: editable.reconfigure(EditorView.editable.of(true)) });
      throw error;
    }
  },
  resume: async () => {
    try {
    if (native) {
      const documents = await Files.list();
      for (const doc of documents) {
        const entry = sessions.get(doc.id);
        if (entry) { entry.note = { ...entry.note, path: doc.path, name: doc.name, dirty: doc.dirty }; if (!doc.dirty) entry.saved = doc.content; }
      }
      const current = sessions.get(note.id);
      if (current) { note = current.note; savedContent = current.saved; }
    }
    updateDocumentUI(); persistRecovery();
    } finally {
      busy = false;
      editor.dispatch({ effects: editable.reconfigure(EditorView.editable.of(true)) });
    }
  },
  discardRecovery: () => { const result = writeRecovery(recoveredDrafts); if (!result.ok) throw Error(result.error); },
} });

// 验证构建向程序化测试提供编辑器入口；正式构建会移除此分支。
if (import.meta.env.MODE === "verification") {
  Object.assign(window, { leafmarkVerification: {
    editor, document: () => ({ ...note }), flush: () => draftQueue,
    undo: () => undo(editor), redo: () => redo(editor),
  } });
}
