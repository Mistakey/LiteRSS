/**
 * 不可信 HTML（RSS 正文、抓到的全文、摘要）进入 DOM 的唯一入口（spec D16）。
 * 正文与摘要两个组件把结果交给 `v-html`，别处不用 `v-html`（eslint 守着）。
 *
 * 顺序：先在惰性文档里把嵌入内容换成外链、把懒加载图片的真实地址换进 src，
 * 再交给 DOMPurify 按白名单清洗；URL 规则在属性钩子里补上 DOMPurify 默认不管的两条：
 * 只收绝对地址，删掉指向应用自身源与回环主机的地址（CSP 挡不住这类 GET，Epic literss-rb0 规划文档 html-sanitize.md 第 4 节）。
 */
import DOMPurify from 'dompurify';

const purify = DOMPurify(window);

const CONFIG = {
  USE_PROFILES: { html: true, mathMl: true },
  FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select', 'option', 'dialog'],
  FORBID_ATTR: ['style', 'id', 'name'],
};

/** 值是 URL 的属性；href、src 与 srcset 另有专门规则。 */
const URL_ATTRS = new Set([
  'href',
  'src',
  'srcset',
  'poster',
  'cite',
  'action',
  'formaction',
  'background',
  'longdesc',
  'ping',
  'xlink:href',
]);

const EMBEDS = 'iframe, embed, object, video, audio';

type UrlKind = 'link' | 'image' | 'plain';

function isLoopback(hostname: string): boolean {
  const h = hostname
    .toLowerCase()
    .replace(/^\[|\]$/g, '')
    .replace(/\.$/, '');
  return (
    h === 'localhost' ||
    h.endsWith('.localhost') ||
    /^(127|0)\.\d+\.\d+\.\d+$/.test(h) ||
    h === '::1' ||
    h === '::' ||
    /^::(ffff:)?(7f[0-9a-f]{2}:|0:)/.test(h)
  );
}

/**
 * 允许时返回要写回的地址，否则 null：绝对 http(s)（不指向自身源或回环），
 * 链接另收 mailto:，图片另收 data:image/*。
 */
function allowedUrl(raw: string, kind: UrlKind): string | null {
  const value = raw.trim();
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return null;
  }
  if (kind === 'link' && url.protocol === 'mailto:') return value;
  if (kind === 'image' && url.protocol === 'data:' && /^data:image\//i.test(value)) return value;
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return null;
  if (url.origin === location.origin || isLoopback(url.hostname)) return null;
  return url.href;
}

const DESCRIPTOR = /^\d+(\.\d+)?[wxh]$/;

/**
 * srcset 按浏览器的切法拆候选（HTML 规范 parse a srcset attribute）：URL 是一段非空白，末尾的逗号结束候选；
 * 否则描述符读到下一个逗号为止。任一 URL 不合规、或描述符不是 `100w`、`2x` 这种形状，就整条删除，
 * 免得 `1x,http://127.0.0.1/…` 这类写法把地址藏进「描述符」。
 */
function allowedSrcset(raw: string): string | null {
  const out: string[] = [];
  let i = 0;
  while (i < raw.length) {
    while (i < raw.length && /[\s,]/.test(raw[i])) i++;
    if (i >= raw.length) break;
    let end = i;
    while (end < raw.length && !/\s/.test(raw[end])) end++;
    let url = raw.slice(i, end);
    i = end;
    let descriptors: string[] = [];
    if (url.endsWith(',')) {
      url = url.replace(/,+$/, '');
    } else {
      const comma = raw.indexOf(',', i);
      const stop = comma < 0 ? raw.length : comma;
      descriptors = raw.slice(i, stop).trim().split(/\s+/).filter(Boolean);
      i = stop + 1;
    }
    const ok = allowedUrl(url, 'image');
    if (!ok || !descriptors.every((d) => DESCRIPTOR.test(d))) return null;
    out.push([ok, ...descriptors].join(' '));
  }
  return out.length ? out.join(', ') : null;
}

purify.addHook('uponSanitizeAttribute', (node, data) => {
  const name = data.attrName;
  if (!URL_ATTRS.has(name)) return;
  let ok: string | null;
  if (name === 'srcset') {
    ok = allowedSrcset(data.attrValue);
  } else {
    const tag = node.nodeName.toLowerCase();
    const kind: UrlKind =
      name === 'href' && tag === 'a' ? 'link' : name === 'src' && tag === 'img' ? 'image' : 'plain';
    ok = allowedUrl(data.attrValue, kind);
  }
  if (ok === null) data.keepAttr = false;
  else data.attrValue = ok;
});

function embedUrl(el: Element): string | null {
  const raw =
    el.getAttribute('src') ||
    el.getAttribute('data') ||
    el.querySelector('source[src]')?.getAttribute('src') ||
    '';
  return raw ? allowedUrl(raw, 'plain') : null;
}

/** 在惰性文档里做清洗前的改写：嵌入内容换成外链，懒加载图片换成真实地址。 */
function prepare(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  for (const el of doc.body.querySelectorAll(EMBEDS)) {
    if (!el.isConnected) continue;
    const url = embedUrl(el);
    if (!url) {
      el.remove();
      continue;
    }
    const a = doc.createElement('a');
    a.className = 'embed-link';
    a.setAttribute('href', url);
    a.textContent = '在浏览器打开嵌入内容';
    el.replaceWith(a);
  }
  for (const img of doc.body.querySelectorAll('img[data-src], img[data-original]')) {
    img.setAttribute('src', img.getAttribute('data-src') || img.getAttribute('data-original')!);
    img.removeAttribute('data-src');
    img.removeAttribute('data-original');
  }
  return doc.body.innerHTML;
}

/** 列表缩略图这类单独进 `<img src>` 的地址按同一条规则过滤；不合规时为空串。 */
export function safeImageUrl(raw: string): string {
  return (raw && allowedUrl(raw, 'image')) || '';
}

export function sanitizeArticleHtml(html: string): string {
  if (!html) return '';
  return purify.sanitize(prepare(html), CONFIG);
}
