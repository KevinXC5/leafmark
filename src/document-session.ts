import type { EditorState } from "@codemirror/state";
import type { Document as NoteDocument, SessionState } from "./mygo";
import { readRecovery, writeRecovery, clearRecovery } from "./session-recovery";

type SessionEntry = { note: NoteDocument; saved: string; state: EditorState };
type SessionOptions = {
  native: boolean;
  initial: NoteDocument;
  files: {
    draft(id: string, content: string): Promise<void>;
    list(): Promise<NoteDocument[]>;
    session(): Promise<SessionState>;
  };
  getState(): EditorState;
  setState(state: EditorState): void;
  createState(content: string): EditorState;
  setEditable(value: boolean): void;
  hasDialog(): boolean;
  autoSave(): boolean;
  save(): Promise<unknown>;
  onWorking(): void;
  onLock(): void;
  onUpdate(): void;
  onError(error: unknown): void;
  setTimer?: typeof setTimeout;
  clearTimer?: typeof clearTimeout;
};

/** 集中维护标签状态、保存基线与草稿队列；界面和窗口事件由入口装配。 */
export function createDocumentSession(options: SessionOptions) {
  const entries = new Map<string, SessionEntry>();
  const externalStates = new Map<string, "changed" | "missing">();
  const setTimer = options.setTimer ?? setTimeout;
  const clearTimer = options.clearTimer ?? clearTimeout;
  let note = options.initial;
  let savedContent = note.content;
  let busy = false;
  let recoveredDrafts = readRecovery();
  let recoveryWarning = "";
  let draftQueue: Promise<unknown> = Promise.resolve();
  let autoSaveTimer: ReturnType<typeof setTimeout> | undefined;
  let recoveryTimer: ReturnType<typeof setTimeout> | undefined;

  function rememberSession() {
    if (note.id) entries.set(note.id, { note: { ...note }, saved: savedContent, state: options.getState() });
  }
  function persistRecovery() {
    rememberSession();
    const result = writeRecovery([...recoveredDrafts, ...[...entries.values()].map(entry => entry.note)]);
    if (!result.ok && result.error !== recoveryWarning) { recoveryWarning = result.error; options.onError(result.error); }
  }
  function cancelAutoSave() { clearTimer(autoSaveTimer); }
  function scheduleAutoSave() {
    cancelAutoSave();
    if (!options.autoSave() || !note.path || !note.dirty || externalStates.has(note.id)) return;
    const id = note.id;
    autoSaveTimer = setTimer(() => {
      if (id !== note.id) return;
      if (busy || options.hasDialog()) { scheduleAutoSave(); return; }
      void perform(options.save);
    }, 2000);
  }
  function queueDraft() {
    note.content = options.getState().doc.toString();
    note.dirty = note.content !== savedContent;
    clearTimer(recoveryTimer); recoveryTimer = setTimer(persistRecovery, 500);
    scheduleAutoSave();
    if (options.native) {
      const { id, content } = note;
      // 串行发送草稿，防止较早的输入后到达后端并覆盖新输入。
      draftQueue = draftQueue.catch(() => {}).then(() => options.files.draft(id, content));
      void draftQueue.catch(options.onError);
    }
  }
  function loadNote(next: NoteDocument) {
    cancelAutoSave();
    rememberSession();
    const existing = entries.get(next.id);
    note = existing ? { ...existing.note, path: next.path, name: next.name } : next;
    savedContent = existing?.saved ?? next.content;
    // 每个标签保留自己的选区和撤销栈，切换不会丢失编辑历史。
    options.setState(existing?.state ?? options.createState(next.content));
  }
  function resetCurrent() { note = { id: "", name: "", path: "", content: "", dirty: false }; }
  function acceptSave(saved: NoteDocument, content: string) {
    savedContent = content;
    externalStates.delete(note.id);
    const current = options.getState().doc.toString();
    note = { ...saved, content: current, dirty: current !== content };
  }
  async function initialize(): Promise<NoteDocument | undefined> {
    if (!options.native) return;
    busy = true;
    options.setEditable(false);
    entries.clear(); note.id = "";
    const session = await options.files.session();
    for (const doc of session.documents) {
      entries.set(doc.id, { note: doc, saved: doc.savedContent, state: options.createState(doc.content) });
    }
    return session.documents.find(doc => doc.id === session.currentId);
  }
  function unlock() { busy = false; options.setEditable(true); }
  async function perform(action: () => Promise<unknown>) {
    if (busy) return;
    busy = true;
    options.onWorking();
    // 原生对话框开启时锁住正文，保持保存快照与屏幕上的文本一致。
    options.setEditable(false);
    options.onLock();
    try { await draftQueue; await action(); }
    catch (error) { options.onError(error); }
    finally { unlock(); options.onUpdate(); }
  }
  const lifecycle = {
    prepareClose: async () => {
      if (busy || options.hasDialog()) throw Error("请先结束当前操作");
      busy = true; cancelAutoSave();
      options.setEditable(false);
      try { persistRecovery(); await draftQueue; }
      catch (error) { unlock(); throw error; }
    },
    resume: async () => {
      try {
        if (options.native) {
          const documents = await options.files.list();
          for (const doc of documents) {
            const entry = entries.get(doc.id);
            if (entry) { entry.note = { ...entry.note, path: doc.path, name: doc.name, dirty: doc.dirty }; if (!doc.dirty) entry.saved = doc.content; }
          }
          const current = entries.get(note.id);
          if (current) { note = current.note; savedContent = current.saved; }
        }
        options.onUpdate(); persistRecovery();
      } finally { unlock(); }
    },
    discardRecovery: () => { const result = writeRecovery(recoveredDrafts); if (!result.ok) throw Error(result.error); },
  };
  return {
    get note() { return note; },
    get busy() { return busy; },
    get recoveredDrafts() { return recoveredDrafts; },
    entries, externalStates, lifecycle,
    rememberSession, persistRecovery, cancelAutoSave, scheduleAutoSave, queueDraft,
    loadNote, resetCurrent, acceptSave, initialize, unlock, perform,
    flush: () => draftQueue,
    removeRecovered: (id: string) => { recoveredDrafts = recoveredDrafts.filter(entry => entry.id !== id); },
    clearRecovered: () => { clearRecovery(); recoveredDrafts = []; },
  };
}

export function safeRecoveryName(name: string) {
  const base = name.split(/[\\/]/).at(-1)?.replace(/[\x00-\x1f<>:\"|?*]/g, "_").replace(/[ .]+$/, "") || "未命名.md";
  if (/^\.(md|markdown)$/i.test(base)) return "未命名.md";
  return /\.(md|markdown)$/i.test(base) ? base : `${base}.md`;
}
