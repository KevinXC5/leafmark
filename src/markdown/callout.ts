import { Info, Lightbulb, TriangleAlert, FileText, CircleCheck, CircleHelp, CircleX, Zap, ListTodo, Quote, Pencil, type IconNode } from "lucide";
import { calloutType } from "./obsidian-syntax";

const calloutIcons: Record<string, IconNode> = {
  note: Info, info: Info, abstract: FileText, todo: ListTodo, tip: Lightbulb,
  success: CircleCheck, question: CircleHelp, warning: TriangleAlert,
  failure: CircleX, danger: Zap, bug: CircleX, example: Pencil, quote: Quote,
};

/** 图标来自固定的 Lucide 数据，标题正文继续交给 Markdown 解析和净化。 */
export function calloutIconHTML(type: string): string {
  const nodes = calloutIcons[calloutType(type)] ?? Info;
  const shapes = nodes.map(([tag, attrs]) => `<${tag} ${Object.entries(attrs).map(([key, value]) => `${key}="${value}"`).join(" ")}></${tag}>`).join("");
  return `<svg class="callout-icon" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${shapes}</svg>`;
}

export const calloutStyles = `
.callout{border:0;border-radius:5px;background:var(--callout-bg,#f1ebe3);color:var(--callout-ink,#3d352f);padding:12px;font:13px/1.5 "Inter",system-ui,sans-serif}
.callout .callout-icon{display:inline-block;width:16px;height:16px;flex-shrink:0;color:var(--callout-accent,#be7559);vertical-align:-3px;margin-right:9px}
.callout .callout-title{font-weight:400}
.callout p{margin:0}
.callout>p+p,.callout>ul,.callout>ol,.callout>pre,.callout>blockquote{margin-top:8px}
.callout .callout-separator{font-weight:400}
`;
