/** 列表行的相对时间；秒级 Unix 时间。 */
export function relativeTime(at: number, now: number): string {
  const min = Math.floor((now - at) / 60);
  if (min < 1) return '刚刚';
  if (min < 60) return `${min} 分钟前`;
  if (min < 1440) return `${Math.floor(min / 60)} 小时前`;
  if (min < 1440 * 30) return `${Math.floor(min / 1440)} 天前`;
  const d = new Date(at * 1000);
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

/** 源的字母徽标取色：按 ID 稳定落到 8 种颜色之一（样式类 c0–c7，不写内联颜色）。 */
export function badgeColor(id: string): string {
  let h = 0;
  for (const ch of id) h = (h * 31 + ch.codePointAt(0)!) >>> 0;
  return `c${h % 8}`;
}

/** 源名的首字，作徽标文字。 */
export function badgeText(title: string): string {
  return Array.from(title.trim())[0]?.toUpperCase() ?? '?';
}

/** 列表显示的标题：有译文用译文（pitfall 12：已判定中文时译文等于原标题）。 */
export function displayTitle(c: { title: string; translated_title: string }): string {
  return c.translated_title || c.title;
}

/** 有与原标题不同的译文：列表打「译」标记，悬停看原标题。 */
export function isTranslated(c: { title: string; translated_title: string }): boolean {
  return c.translated_title !== '' && c.translated_title !== c.title;
}

/** 已判定为中文：只有这时才显示 RSS 摘录（spec D15「一行中文摘录」）。 */
export function isChinese(c: { title: string; translated_title: string }): boolean {
  return c.translated_title !== '' && c.translated_title === c.title;
}
