/**
 * 正文够不够摘要（spec D11）：与后端门槛一致，只数可见文字。
 * 输入是不可信 HTML，只在惰性文档里读文字，不进页面。
 */

/** 与后端 enrich.minSummaryRunes 一致：正文至少这么多可见字才会摘要。 */
const MIN_SUMMARY_CHARS = 300;

/** 非空白 Unicode 字符数，与后端 htmltext.Visible 的数法相同。 */
export function visibleLength(html: string): number {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  doc.body.querySelectorAll('script, style, noscript, template').forEach((el) => el.remove());
  let n = 0;
  for (const ch of doc.body.textContent ?? '') if (!/\s/u.test(ch)) n++;
  return n;
}

export function longEnoughToSummarize(html: string): boolean {
  return visibleLength(html) >= MIN_SUMMARY_CHARS;
}
