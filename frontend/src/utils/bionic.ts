/**
 * 正文的 Bionic Reading（spec D22）：加粗每个英文词的开头。
 * 只改已渲染正文的文本节点，加粗标记由 DOM API 生成，不经 HTML 解析（spec D16）；
 * 代码、公式、小标题、原本就加粗的文字和对照译文不动，汉字不分词，所以中文不变。
 */
import { TRANSLATION_CLASS } from './bilingual';

// 词长查表照抄 text-vide 的 getFixationLength，只取 fixation 1 档（模仿官方 API 的默认输出）。
// text-vide: https://github.com/Gumball12/text-vide（packages/text-vide/src/getFixationLength.ts）
// Copyright (c) 2022 shj, MIT License
const FIXATION_BOUNDARY = [0, 4, 12, 17, 24, 29, 35, 42, 48];

/** 长度为 len 的词开头要加粗的字母数。 */
export function fixationLength(len: number): number {
  const fromLast = FIXATION_BOUNDARY.findIndex((b) => len <= b);
  return Math.max(fromLast === -1 ? len - FIXATION_BOUNDARY.length : len - fromLast, 0);
}

// 公式连同 enhance 的外壳一起跳过：KaTeX 渲染失败时外壳里留的是 TeX 源码。
const SKIP = `pre, code, math, .katex, .katex-display, .katex-inline, h1, h2, h3, h4, h5, h6, b, strong, .${TRANSLATION_CLASS}`;

// 与 text-vide 相同：字母与数字组成、至少含一个字母；只是汉字不算字母。
const NON_HAN_LETTER = '(?:(?!\\p{Script=Han})\\p{L})';
const WORD = new RegExp(
  `(?:${NON_HAN_LETTER}|\\p{Nd})*${NON_HAN_LETTER}(?:${NON_HAN_LETTER}|\\p{Nd})*`,
  'gu'
);
const HAS_WORD = new RegExp(NON_HAN_LETTER, 'u');

function skipped(node: Node, root: HTMLElement) {
  const hit = node.parentElement?.closest(SKIP);
  return !!hit && root.contains(hit);
}

function emphasize(text: Text) {
  const s = text.data;
  const parts: (string | Node)[] = [];
  let at = 0;
  for (const m of s.matchAll(WORD)) {
    const n = fixationLength(m[0].length);
    if (!n) continue;
    const head = document.createElement('span');
    head.className = 'bionic-fix';
    head.textContent = m[0].slice(0, n);
    parts.push(s.slice(at, m.index), head);
    at = m.index + n;
  }
  if (!parts.length) return;
  parts.push(s.slice(at));
  text.replaceWith(...parts.filter((p) => p !== ''));
}

/** 给 root 里的正文加粗词首；对同一份 DOM 只能调用一次，要重来就重建正文。 */
export function applyBionic(root: HTMLElement) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (node) =>
      HAS_WORD.test(node.nodeValue ?? '') && !skipped(node, root)
        ? NodeFilter.FILTER_ACCEPT
        : NodeFilter.FILTER_REJECT,
  });
  const texts: Text[] = [];
  while (walker.nextNode()) texts.push(walker.currentNode as Text);
  texts.forEach(emphasize);
}
