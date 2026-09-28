/**
 * 正文的两个判定（spec D11）：点开时要不要自动抓全文，全文抓不到时 RSS 正文够不够摘要。
 * 输入是不可信 HTML，只在惰性文档里读文字，不进页面。
 */

/** 可见文字少于这个数就当作截断，点开时自动抓全文。 */
const TRUNCATED_BELOW = 500;

/** 与后端 enrich.minSummaryRunes 一致：RSS 正文至少这么多可见字才会基于它摘要。 */
const MIN_SUMMARY_CHARS = 300;

/** 正文结尾的「未完」标记：省略号与各种「阅读全文」。 */
const TAIL_MARKER =
  /(…|\.\.\.|\[\s*(…|\.\.\.)\s*\]|read\s+more|continue\s+reading|keep\s+reading|阅读全文|查看全文|阅读更多|继续阅读)\W*$/iu;

function textOf(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  doc.body.querySelectorAll('script, style, noscript, template').forEach((el) => el.remove());
  return doc.body.textContent ?? '';
}

function countVisible(text: string): number {
  let n = 0;
  for (const ch of text) if (!/\s/u.test(ch)) n++;
  return n;
}

/** 非空白 Unicode 字符数，与后端 htmltext.Visible 的数法相同。 */
export function visibleLength(html: string): number {
  return countVisible(textOf(html));
}

/**
 * RSS 正文像是被截断了：文字很少，或以「未完」标记结尾。带题图的首段摘录同样算截断；
 * 误判的代价只是多抓一次全文（成功才替换，失败保留原文）。
 */
export function isTruncated(html: string): boolean {
  const text = textOf(html);
  if (TAIL_MARKER.test(text.trim())) return true;
  return countVisible(text) < TRUNCATED_BELOW;
}

export function longEnoughToSummarize(html: string): boolean {
  return visibleLength(html) >= MIN_SUMMARY_CHARS;
}
