import { syntaxTree } from "@codemirror/language";
import type { EditorState, Text } from "@codemirror/state";

export interface DocumentHeading {
  level: number;
  title: string;
  from: number;
  to: number;
}

export interface DocumentStats {
  wordCount: number;
  readMinutes: number;
  headings: DocumentHeading[];
}

const wordCounts = new WeakMap<Text, number>();
const frontmatterEnds = new WeakMap<Text, number>();
const analyses = new WeakMap<Text, { tree: ReturnType<typeof syntaxTree>; stats: DocumentStats }>();
const han = /\p{Script=Han}/u;
const letterOrNumber = /[\p{L}\p{N}]/u;
const combiningMark = /\p{M}/u;

function countWords(doc: Text): number {
  const cached = wordCounts.get(doc);
  if (cached !== undefined) return cached;
  let count = 0;
  if (doc.children) {
    // Text 子节点之间总有换行；复用未变化子树时无需合并跨节点单词。
    for (const child of doc.children) count += countWords(child);
  } else {
    let inWord = false;
    for (const chunk of doc.iter()) {
      // 键帽 emoji 含 ASCII 数字，不能把它的基字符当作数字词。
      const text = chunk.includes("⃣") ? chunk.replace(/[0-9#*]️?⃣/g, " ") : chunk;
      for (const char of text) {
        const code = char.codePointAt(0)!;
        if (code < 128) {
          const isWord = (code >= 48 && code <= 57) || (code >= 65 && code <= 90) || (code >= 97 && code <= 122);
          if (isWord && !inWord) count++;
          inWord = isWord;
        } else if (han.test(char)) {
          count++;
          inWord = false;
        } else if (letterOrNumber.test(char)) {
          if (!inWord) count++;
          inWord = true;
        } else if (!combiningMark.test(char)) {
          inWord = false;
        }
      }
    }
  }
  wordCounts.set(doc, count);
  return count;
}

function frontmatterEnd(doc: Text): number {
  const cached = frontmatterEnds.get(doc);
  if (cached !== undefined) return cached;
  let end = 0;
  if (/^﻿?---\s*$/.test(doc.line(1).text)) {
    let position = 0;
    let firstLine = true;
    for (const line of doc.iterLines()) {
      position += line.length + 1;
      if (firstLine) { firstLine = false; continue; }
      if (/^(?:---|\.\.\.)\s*$/.test(line)) { end = Math.min(position, doc.length); break; }
    }
  }
  frontmatterEnds.set(doc, end);
  return end;
}

function headingTitle(source: string, atx: boolean): string {
  const text = atx
    ? source.replace(/^#{1,6}(?:\s+|$)/, "").replace(/\s+#+\s*$/, "")
    : source.replace(/\n[^\n]*$/, "");
  return text
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/!?\[([^\]]*)\]\[[^\]]*\]/g, "$1")
    .replace(/<[^>]*>/g, "")
    .replace(/\\([\\`*_{}\[\]()#+.!~>\-])/g, "$1")
    .replace(/[*_`~]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

/**
 * 统计原始 Markdown 字数：汉字逐字计数，其他文字和数字按连续词计数。
 * 大纲只使用已有语法树，不强制补解析；解析推进后再次调用即可更新。
 * 返回值按文本和语法树缓存，调用方应将其视为只读。
 */
export function analyzeDocument(state: EditorState): DocumentStats {
  const doc = state.doc;
  const tree = syntaxTree(state);
  const cached = analyses.get(doc);
  if (cached?.tree === tree) return cached.stats;
  const wordCount = countWords(doc);
  const headings: DocumentHeading[] = [];
  const metadataEnd = frontmatterEnd(doc);
  tree.iterate({ enter(node) {
    if (node.to <= metadataEnd) return false;
    const match = /^(ATX|Setext)Heading([1-6])$/.exec(node.name);
    if (match) {
      if (node.from >= metadataEnd) headings.push({
        level: Number(match[2]),
        title: headingTitle(doc.sliceString(node.from, node.to), match[1] === "ATX"),
        from: node.from,
        to: node.to,
      });
      return false;
    }
    // 大纲只需要块级结构，不遍历正文内的链接、强调和嵌入代码语言树。
    if (/^(?:Paragraph|FencedCode|CodeBlock|LinkReference|HTMLBlock|Table|HorizontalRule)$/.test(node.name)) return false;
  } });
  const stats = { wordCount, readMinutes: Math.max(1, Math.ceil(wordCount / 250)), headings };
  analyses.set(doc, { tree, stats });
  return stats;
}
