/**
 * 正文与摘要里的链接点击：窗口本身从不跳转。http(s) 返回地址，由调用方经 `POST /api/browser/open`
 * 交给系统浏览器；`mailto:` 不拦，交给系统；其余（清洗后本不该剩下）只拦不开。
 */
export function interceptLink(e: MouseEvent): string | null {
  const href = (e.target as Element).closest('a')?.getAttribute('href');
  if (!href || /^mailto:/i.test(href)) return null;
  e.preventDefault();
  return /^https?:/i.test(href) ? href : null;
}
