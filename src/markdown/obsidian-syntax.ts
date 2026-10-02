export interface WikiReference {
  length: number;
  target: string;
  label: string;
  embed: boolean;
  image: boolean;
  width?: number;
  height?: number;
}

/** Wiki 目标始终按笔记路径编码，不能被解释成脚本协议或网络地址。 */
export function wikiHref(target: string): string {
  const hash = target.indexOf("#");
  const path = hash < 0 ? target : target.slice(0, hash);
  const fragment = hash < 0 ? "" : target.slice(hash + 1);
  return (path ? "./" + path.split("/").map(encodeURIComponent).join("/") : "") + (hash < 0 ? "" : "#" + encodeURIComponent(fragment));
}

/** 两种渲染器共享完整 wiki 边界，避免普通 Markdown 只消费其中一对方括号。 */
export function parseWikiReference(source: string, from = 0): WikiReference | undefined {
  const match = source.slice(from).match(/^(!?)\[\[([^\[\]\n\r]+)\]\]/);
  if (!match) return;
  const [targetPart, ...aliasParts] = match[2]!.split("|");
  const target = targetPart!.trim();
  if (!target || target === "#") return;
  const alias = aliasParts.join("|").trim();
  const embed = match[1] === "!";
  const image = embed && /\.(?:png|jpe?g|gif|webp|svg|avif|bmp)$/i.test(target.split("#")[0]!);
  const dimensions = image ? alias.match(/^(\d+)(?:x(\d+))?$/) : null;
  return {
    length: match[0].length, target, embed, image,
    label: dimensions ? target : alias || target.replace(/^#/, ""),
    width: dimensions ? Math.min(4096, Math.max(1, Number(dimensions[1]))) : undefined,
    height: dimensions?.[2] ? Math.min(4096, Math.max(1, Number(dimensions[2]))) : undefined,
  };
}

export const calloutPattern = /^\[!([A-Za-z][A-Za-z\d_-]*)\]([+-])?(?:[ \t]+(.*))?$/;
const calloutAliases: Record<string, string> = { summary: "abstract", tldr: "abstract", hint: "tip", important: "tip", check: "success", done: "success", help: "question", faq: "question", caution: "warning", attention: "warning", fail: "failure", missing: "failure", error: "danger", cite: "quote" };
export function calloutType(type: string): string {
  const lower = type.toLowerCase();
  return calloutAliases[lower] || lower;
}
