export type RecoveredNote = { id: string; name: string; path: string; content: string; dirty: boolean };
export const RECOVERY_STORAGE_KEY = "leafmark-recovery-v1";
export const MAX_RECOVERY_NOTE_BYTES = 1024 * 1024;
export const MAX_RECOVERY_BYTES = 5 * 1024 * 1024;
export type RecoveryWriteResult = { ok: true } | { ok: false; error: string };

const encoder = new TextEncoder();
const bytes = (value: string) => encoder.encode(value).byteLength;
function isNote(value: unknown): value is RecoveredNote {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const note = value as Record<string, unknown>;
  return typeof note.id === "string" && note.id.length > 0 && typeof note.name === "string"
    && typeof note.path === "string" && typeof note.content === "string" && typeof note.dirty === "boolean";
}
function snapshot(note: RecoveredNote): RecoveredNote {
  return { id: note.id, name: note.name, path: note.path, content: note.content, dirty: note.dirty };
}

/** 损坏记录逐条跳过；所有内容均视为文本，调用方应安全渲染。 */
export function readRecovery(): RecoveredNote[] {
  try {
    const raw = localStorage.getItem(RECOVERY_STORAGE_KEY);
    if (!raw || bytes(raw) > MAX_RECOVERY_BYTES) return [];
    const data: unknown = JSON.parse(raw);
    if (!Array.isArray(data)) return [];
    const seen = new Set<string>();
    return data.filter((value): value is RecoveredNote => {
      if (!isNote(value) || !value.dirty || bytes(value.content) > MAX_RECOVERY_NOTE_BYTES
        || bytes(JSON.stringify(snapshot(value))) > MAX_RECOVERY_NOTE_BYTES || seen.has(value.id)) return false;
      seen.add(value.id); return true;
    }).map(snapshot);
  } catch { return []; }
}

/** 超限或存储失败时保留旧快照，并明确返回错误，避免无提示丢失草稿。 */
export function writeRecovery(notes: RecoveredNote[]): RecoveryWriteResult {
  try {
    if (!Array.isArray(notes) || notes.some(note => !isNote(note))) return { ok: false, error: "草稿数据格式无效，恢复副本未更新。" };
    const dirty = notes.filter(note => note.dirty).map(snapshot);
    const seen = new Set<string>();
    for (const note of dirty) {
      if (seen.has(note.id)) return { ok: false, error: "草稿标识重复，恢复副本未更新。" };
      seen.add(note.id);
      // 按 UTF-8 字节检查整个记录，计入名称、路径及 JSON 转义开销。
      if (bytes(note.content) > MAX_RECOVERY_NOTE_BYTES || bytes(JSON.stringify(note)) > MAX_RECOVERY_NOTE_BYTES) {
        return { ok: false, error: "单份草稿超过 1 MB，恢复副本未更新。请保存到本地文件。" };
      }
    }
    const serialized = JSON.stringify(dirty);
    if (bytes(serialized) > MAX_RECOVERY_BYTES) return { ok: false, error: "恢复草稿总量超过 5 MB，恢复副本未更新。请保存到本地文件。" };
    localStorage.setItem(RECOVERY_STORAGE_KEY, serialized);
    return { ok: true };
  } catch {
    return { ok: false, error: "无法保存恢复草稿：本地存储不可用或空间不足。请保存到本地文件。" };
  }
}

/** 删除失败抛出错误，供调用方展示；仅操作本模块的 key。 */
export function clearRecovery(): void {
  try { localStorage.removeItem(RECOVERY_STORAGE_KEY); }
  catch { throw new Error("无法清理恢复草稿：本地存储不可用。"); }
}

export function removeRecovery(id: string): RecoveryWriteResult {
  // 读取失败不能当作空列表再写回，否则会覆盖仍在存储中的恢复副本。
  try {
    const raw = localStorage.getItem(RECOVERY_STORAGE_KEY);
    if (raw === null) return { ok: true };
    if (bytes(raw) > MAX_RECOVERY_BYTES) return { ok: false, error: "恢复草稿超过容量限制，无法移除指定草稿。" };
    const data: unknown = JSON.parse(raw);
    if (!Array.isArray(data) || data.some(note => !isNote(note))) return { ok: false, error: "恢复草稿数据损坏，无法移除指定草稿。" };
    return writeRecovery(data.filter(note => note.id !== id));
  } catch { return { ok: false, error: "无法移除恢复草稿：本地存储不可用或数据损坏。" }; }
}
