/**
 * 清洗之后对正文做的增强：公式（KaTeX）与代码高亮（highlight.js）。它们生成的标记可信，所以放在清洗之后，
 * 样式经 CSSOM 写入，不受 CSP `style-src 'self'` 限制（Epic literss-rb0 规划文档 html-sanitize.md 第 5 节）。
 *
 * 这个模块连同 KaTeX、highlight.js 按需载入（ArticleBody 动态 import），不进首屏包。
 *
 * FreshRSS 把 class 改名成 data-sanitized-class，readability 会删掉 class，所以两处都读（spec D16）。
 */
import katex from 'katex';
import 'katex/dist/katex.min.css';
import hljs from 'highlight.js/lib/core';
import bash from 'highlight.js/lib/languages/bash';
import c from 'highlight.js/lib/languages/c';
import cpp from 'highlight.js/lib/languages/cpp';
import csharp from 'highlight.js/lib/languages/csharp';
import css from 'highlight.js/lib/languages/css';
import diff from 'highlight.js/lib/languages/diff';
import go from 'highlight.js/lib/languages/go';
import java from 'highlight.js/lib/languages/java';
import javascript from 'highlight.js/lib/languages/javascript';
import json from 'highlight.js/lib/languages/json';
import kotlin from 'highlight.js/lib/languages/kotlin';
import markdown from 'highlight.js/lib/languages/markdown';
import php from 'highlight.js/lib/languages/php';
import plaintext from 'highlight.js/lib/languages/plaintext';
import powershell from 'highlight.js/lib/languages/powershell';
import python from 'highlight.js/lib/languages/python';
import ruby from 'highlight.js/lib/languages/ruby';
import rust from 'highlight.js/lib/languages/rust';
import sql from 'highlight.js/lib/languages/sql';
import swift from 'highlight.js/lib/languages/swift';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import yaml from 'highlight.js/lib/languages/yaml';

const languages = {
  bash,
  c,
  cpp,
  csharp,
  css,
  diff,
  go,
  java,
  javascript,
  json,
  kotlin,
  markdown,
  php,
  plaintext,
  powershell,
  python,
  ruby,
  rust,
  sql,
  swift,
  typescript,
  xml,
  yaml,
};
for (const [name, lang] of Object.entries(languages)) hljs.registerLanguage(name, lang);
hljs.registerAliases(['sh', 'shell', 'zsh'], { languageName: 'bash' });
hljs.registerAliases(['js', 'jsx'], { languageName: 'javascript' });
hljs.registerAliases(['ts', 'tsx'], { languageName: 'typescript' });
hljs.registerAliases(['py'], { languageName: 'python' });
hljs.registerAliases(['rs'], { languageName: 'rust' });
hljs.registerAliases(['html', 'svg'], { languageName: 'xml' });
hljs.registerAliases(['yml'], { languageName: 'yaml' });
hljs.registerAliases(['text', 'txt'], { languageName: 'plaintext' });

/** 元素的类名：原 class 与 FreshRSS 改名后的 data-sanitized-class。 */
function classesOf(el: Element): string[] {
  return `${el.getAttribute('class') ?? ''} ${el.getAttribute('data-sanitized-class') ?? ''}`
    .split(/\s+/)
    .filter(Boolean);
}

function renderTex(tex: string, display: boolean): HTMLElement {
  const el = document.createElement(display ? 'div' : 'span');
  el.className = display ? 'katex-display' : 'katex-inline';
  katex.render(tex, el, { displayMode: display, throwOnError: false, strict: false });
  return el;
}

// 长的、明确的定界符在前；行内 $…$ 两端不能是空白，避免把「$5 和 $10」当公式。
const DELIMITERS: { re: RegExp; display: boolean }[] = [
  { re: /\$\$([^$]+)\$\$/g, display: true },
  { re: /\\\[([\s\S]+?)\\\]/g, display: true },
  { re: /\\\(([\s\S]+?)\\\)/g, display: false },
  { re: /\$([^\s$](?:[^$\n]*[^\s$])?)\$/g, display: false },
];

function renderTextMath(text: Text) {
  const s = text.data;
  const found: { start: number; end: number; tex: string; display: boolean }[] = [];
  for (const { re, display } of DELIMITERS) {
    for (const m of s.matchAll(re)) {
      const start = m.index;
      const end = start + m[0].length;
      if (found.some((f) => start < f.end && end > f.start)) continue;
      found.push({ start, end, tex: m[1].trim(), display });
    }
  }
  if (!found.length) return;
  found.sort((a, b) => a.start - b.start);
  const parts: (string | Node)[] = [];
  let at = 0;
  for (const f of found) {
    parts.push(s.slice(at, f.start), renderTex(f.tex, f.display));
    at = f.end;
  }
  parts.push(s.slice(at));
  text.replaceWith(...parts.filter((p) => p !== ''));
}

export function renderMath(root: HTMLElement) {
  for (const el of root.querySelectorAll('[data-math]')) {
    el.replaceWith(renderTex(el.getAttribute('data-math')!, el.tagName === 'DIV'));
  }
  for (const el of root.querySelectorAll('*')) {
    const cls = classesOf(el);
    if (!cls.includes('math') || !root.contains(el)) continue;
    el.replaceWith(
      renderTex(el.textContent ?? '', cls.includes('display') || el.tagName === 'DIV')
    );
  }
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (node) =>
      node.parentElement?.closest('pre, code, .katex, .katex-display, .katex-inline, math')
        ? NodeFilter.FILTER_REJECT
        : /\$|\\\(|\\\[/.test(node.textContent ?? '')
          ? NodeFilter.FILTER_ACCEPT
          : NodeFilter.FILTER_REJECT,
  });
  const texts: Text[] = [];
  while (walker.nextNode()) texts.push(walker.currentNode as Text);
  texts.forEach(renderTextMath);
}

function languageOf(el: Element): string | undefined {
  for (const node of [el, el.parentElement]) {
    if (!node) continue;
    for (const cls of classesOf(node)) {
      const m = cls.match(/^(?:language|lang)-(.+)$/);
      if (m && hljs.getLanguage(m[1])) return m[1];
    }
  }
  return undefined;
}

export function highlightCode(root: HTMLElement) {
  const blocks = [
    ...root.querySelectorAll('pre > code'),
    ...[...root.querySelectorAll('pre')].filter((pre) => !pre.querySelector('code')),
  ];
  for (const block of blocks) {
    if (block.classList.contains('hljs')) continue;
    const code = block.textContent ?? '';
    const language = languageOf(block);
    const result = language ? hljs.highlight(code, { language }) : hljs.highlightAuto(code);
    block.innerHTML = result.value;
    block.classList.add('hljs');
  }
}

/** 对已插入页面的正文做全部增强；出错时保留原样，不影响阅读。 */
export function enhance(root: HTMLElement) {
  try {
    renderMath(root);
  } catch {
    // 公式渲染失败时显示原文
  }
  try {
    highlightCode(root);
  } catch {
    // 高亮失败时显示原文
  }
}
