import type { EditorState, Transaction } from "@codemirror/state";
import { getSearchQuery, searchPanelOpen, setSearchQuery } from "@codemirror/search";

/** 查找面板开启且范围内有匹配时返回 true：预览让出位置显示源文，匹配才能被高亮和替换。 */
export function searchReveals(state: EditorState, from: number, to: number): boolean {
  if (!searchPanelOpen(state)) return false;
  const query = getSearchQuery(state);
  if (!query.valid || !query.search) return false;
  return !query.getCursor(state, from, to).next().done;
}

/** 查询内容或面板开合变化时，预览装饰需要重新计算。 */
export function searchChanged(transaction: Transaction): boolean {
  return transaction.effects.some(effect => effect.is(setSearchQuery)) || searchPanelOpen(transaction.startState) !== searchPanelOpen(transaction.state);
}
