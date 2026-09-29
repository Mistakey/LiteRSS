/**
 * 全文翻译的中英段落对照（spec D21）。
 *
 * 块级文本是段落、列表项、小标题、引用、图注；代码块、表格、公式里的不翻，图片本来就没有文字。
 * 块从已清洗的正文里取：store 用 textBlocks 取出要翻的文字，ArticleBody 用 interleave 在同一份
 * HTML 的同样的块里插入译文，所以两边按下标一一对应。
 */

const BLOCK = 'p, li, h1, h2, h3, h4, h5, h6, blockquote, figcaption';
/** 其中的文字原样保留、不翻。 */
const KEEP = 'pre, table, math, svg';
/** 插入的译文元素的类名；Bionic Reading 据此跳过译文。 */
export const TRANSLATION_CLASS = 'literss-tr';

interface Block {
  el: Element;
  text: string;
}

function blocksOf(root: DocumentFragment): Block[] {
  const out: Block[] = [];
  for (const el of root.querySelectorAll(BLOCK)) {
    if (el.closest(KEEP)) continue;
    const text = ownText(el).replace(/\s+/g, ' ').trim();
    // 只有数字、符号的块（例如「* * *」）没有可翻的。
    if (/\p{L}/u.test(text)) out.push({ el, text });
  }
  return out;
}

/** 块自己的文字：不含嵌套块（它们自成一块）与 KEEP 里的内容。 */
function ownText(el: Element): string {
  let text = '';
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const parent = n.parentElement!;
    if (parent.closest(BLOCK) === el && !parent.closest(KEEP)) text += n.textContent;
  }
  return text;
}

function parse(html: string): DocumentFragment {
  const t = document.createElement('template');
  t.innerHTML = html;
  return t.content;
}

/** 已清洗正文里要翻译的块文字，按文档顺序。 */
export function textBlocks(safeHtml: string): string[] {
  return blocksOf(parse(safeHtml)).map((b) => b.text);
}

/**
 * 在每个块的原文下插入它的译文：纯文本，经 textContent 写入，不当 HTML 解析（spec D21）。
 * 译文放在块内的末尾，块里有嵌套块（如带子列表的列表项）时放在第一个嵌套块之前，紧跟这块自己的文字。
 * 块数与译文数对不上时原样返回。
 */
export function interleave(safeHtml: string, translations: string[]): string {
  const root = parse(safeHtml);
  const blocks = blocksOf(root);
  if (blocks.length !== translations.length) return safeHtml;
  blocks.forEach(({ el }, i) => {
    const tr = document.createElement('span');
    tr.className = TRANSLATION_CLASS;
    tr.lang = 'zh-CN';
    tr.textContent = translations[i];
    const nested = [...el.childNodes].find(
      (n) => n instanceof Element && (n.matches(BLOCK) || n.querySelector(BLOCK))
    );
    el.insertBefore(tr, nested ?? null);
  });
  const t = document.createElement('template');
  t.content.append(root);
  return t.innerHTML;
}
