import { describe, expect, it } from 'vitest';
import { safeImageUrl, sanitizeArticleHtml } from './sanitize';

// 载荷矩阵：Epic literss-rb0 规划文档 html-sanitize.md 第 1、2、3、4 节（git show 4056ac16:docs/specs/literss-rb0/html-sanitize.md）。
function dom(html: string) {
  const div = document.createElement('div');
  div.innerHTML = sanitizeArticleHtml(html);
  return div;
}

/** 清洗结果里不应出现的东西：任何事件属性、脚本类 URL、嵌入元素。 */
function assertInert(div: HTMLElement) {
  for (const el of div.querySelectorAll('*')) {
    for (const attr of el.attributes) {
      expect(attr.name, el.outerHTML).not.toMatch(/^on/i);
      expect(attr.name, el.outerHTML).not.toBe('srcdoc');
      expect(attr.value.replace(/\s/g, ''), el.outerHTML).not.toMatch(
        /^(javascript|vbscript|data:text)/i
      );
    }
  }
  expect(
    div.querySelector('script, iframe, embed, object, style, link, meta, base, form')
  ).toBeNull();
}

describe('第 1 节：readability 与 FreshRSS 留下的元素', () => {
  it.each([
    '<script>fetch("/api/x")</script>',
    '<noscript><img src="https://a.example/x.png"></noscript>',
    '<style>body{display:none}</style>',
    '<link rel="stylesheet" href="https://a.example/x.css">',
    '<meta http-equiv="refresh" content="0;url=https://a.example">',
    '<base href="https://evil.example/">',
    '<form action="https://evil.example"><input name="q"><button>go</button></form>',
    '<p onclick="fetch(1)" onmouseover="fetch(2)">x</p>',
    '<img src="https://a.example/x.png" onerror="fetch(1)">',
    '<details open ontoggle="fetch(1)"><summary>s</summary></details>',
    '<video src="https://a.example/v.mp4" onloadstart="fetch(1)"></video>',
    '<svg><animate onbegin="fetch(1)" attributeName="x" dur="1s"></animate></svg>',
    '<a href="JavaScript:fetch(1)">x</a>',
    '<a href=" javascript:fetch(1)">x</a>',
    '<a href="java&#x09;script:fetch(1)">x</a>',
    '<a href="data:text/html,<script>fetch(1)</script>">x</a>',
    '<iframe src="https://www.youtube.com/embed/x"></iframe>',
    '<embed src="https://www.youtube.com/v/x">',
    '<object data="https://www.youtube.com/v/x"></object>',
    '<iframe srcdoc="<script>parent.fetch(1)</script>" title="//www.youtube.com/embed/y"></iframe>',
    '<iframe src="https://evil.example/?ref=//www.youtube.com/embed/x"></iframe>',
    '<math><mi xlink:href="javascript:fetch(1)">x</mi></math>',
  ])('%s', (html) => {
    assertInert(dom(html));
  });

  it('去掉 style、id、name 属性', () => {
    const div = dom('<p style="position:fixed" id="x" name="y" class="c">t</p>');
    const p = div.querySelector('p')!;
    expect(p.getAttribute('style')).toBeNull();
    expect(p.getAttribute('id')).toBeNull();
    expect(p.getAttribute('name')).toBeNull();
    expect(p.textContent).toBe('t');
  });
});

describe('第 2 节：未清洗时能执行的载荷', () => {
  it.each([
    '<img src=x onerror="fetch(\'/api/hit\')">',
    '<details ontoggle="fetch(\'/api/hit\')" open>x</details>',
    '<svg><animate onbegin="fetch(\'/api/hit\')" attributeName="x" dur="1s"/></svg>',
    '<video onloadstart="fetch(\'/api/hit\')"><source src="https://a.example/v.mp4"></video>',
    '<iframe srcdoc="<script>parent.fetch(\'/api/hit\')</script>"></iframe>',
    '<a href="JavaScript:fetch(\'/api/hit\')">x</a>',
    '<a href=" javascript:fetch(\'/api/hit\')">x</a>',
  ])('%s', (html) => {
    assertInert(dom(html));
  });
});

describe('第 3 节：摘要里模型输出的 HTML', () => {
  it.each([
    "<img src=x onerror=fetch('/api/articles/mark-all-read')>",
    '<svg/onload=alert(1)>',
    '<a href="java&#115;cript:alert(1)">x</a>',
  ])('%s', (html) => {
    assertInert(dom(html));
  });
});

describe('第 4 节：指向自身源与回环主机的地址', () => {
  it.each([
    '/api/hit?x=1',
    'api/hit',
    `${location.origin}/api/hit`,
    'http://127.0.0.1:18731/api/hit',
    'http://127.1/api/hit',
    'http://2130706433/api/hit',
    'http://localhost:1235/api/hit',
    'http://wails.localhost/api/articles/read',
    'http://[::1]:1235/api/hit',
    'http://[::127.0.0.1]/api/hit',
    'http://[::ffff:127.0.0.1]/api/hit',
    'http://0.0.0.0:1235/api/hit',
  ])('%s', (url) => {
    const div = dom(
      `<img src="${url}"><a href="${url}">a</a><img srcset="https://a.example/1.png 1x, ${url} 2x">`
    );
    expect(div.querySelector('img[src]')).toBeNull();
    expect(div.querySelector('a[href]')).toBeNull();
    expect(div.querySelector('img[srcset]')).toBeNull();
  });
});

describe('srcset 按浏览器的切分检查每个候选', () => {
  it.each([
    'https://a.example/1.png 1x,http://127.0.0.1:1235/api/hit 2x',
    'https://a.example/1.png 1x ,/api/hit 2x',
    'https://a.example/1.png, /api/hit',
    'https://a.example/1.png 1x http://127.0.0.1/api/hit',
  ])('%s', (srcset) => {
    expect(dom(`<img srcset="${srcset}">`).querySelector('img[srcset]')).toBeNull();
  });

  it('合规的写法保留，URL 里的逗号不切', () => {
    const div = dom(
      '<img srcset="https://img.example/w_100,h_50/a.png 100w,https://img.example/b.png 2.5x">'
    );
    expect(div.querySelector('img')!.getAttribute('srcset')).toBe(
      'https://img.example/w_100,h_50/a.png 100w, https://img.example/b.png 2.5x'
    );
  });
});

describe('应保留的', () => {
  it('mailto: 链接', () => {
    expect(dom('<a href="mailto:a@b.example">m</a>').querySelector('a')!.getAttribute('href')).toBe(
      'mailto:a@b.example'
    );
  });

  it('FreshRSS 改名后的 data-sanitized-class 与 class', () => {
    const div = dom(
      '<pre><code data-sanitized-class="language-go" class="language-go">x</code></pre>'
    );
    const code = div.querySelector('code')!;
    expect(code.getAttribute('data-sanitized-class')).toBe('language-go');
    expect(code.className).toBe('language-go');
  });

  it('外部图片、http(s) 链接与 data:image', () => {
    const div = dom(
      '<img src="https://img.example/a.png" srcset="https://img.example/a.png 1x, https://img.example/b.png 2x">' +
        '<a href="http://site.example/p">p</a><img src="data:image/png;base64,iVBORw0KGgo=">'
    );
    const [ext, data] = div.querySelectorAll('img');
    expect(ext.getAttribute('src')).toBe('https://img.example/a.png');
    expect(ext.getAttribute('srcset')).toContain('b.png 2x');
    expect(data.getAttribute('src')).toMatch(/^data:image\/png/);
    expect(div.querySelector('a')!.getAttribute('href')).toBe('http://site.example/p');
  });

  it('mailto 与 data: 只在各自的位置有效', () => {
    const div = dom('<img src="mailto:a@b.example"><a href="data:image/png;base64,AAAA">x</a>');
    expect(div.querySelector('img[src]')).toBeNull();
    expect(div.querySelector('a[href]')).toBeNull();
  });

  it('MathML 公式', () => {
    const div = dom('<math><mi>x</mi><mo>+</mo><mn>1</mn></math>');
    expect(div.querySelector('math mi')!.textContent).toBe('x');
  });
});

describe('嵌入内容改成外链', () => {
  it.each([
    ['<iframe src="https://www.youtube.com/embed/x"></iframe>', 'https://www.youtube.com/embed/x'],
    ['<embed src="https://a.example/e.swf">', 'https://a.example/e.swf'],
    ['<object data="https://a.example/o"></object>', 'https://a.example/o'],
    ['<video src="https://a.example/v.mp4" controls></video>', 'https://a.example/v.mp4'],
    ['<audio><source src="https://a.example/a.mp3"></audio>', 'https://a.example/a.mp3'],
  ])('%s', (html, url) => {
    const div = dom(html);
    const a = div.querySelector('a.embed-link')!;
    expect(a.getAttribute('href')).toBe(url);
    expect(a.textContent).toBe('在浏览器打开嵌入内容');
    expect(div.querySelector('video, audio, source')).toBeNull();
  });

  it('非 http(s) 或指向回环的嵌入内容整个删除', () => {
    const div = dom(
      '<iframe src="javascript:fetch(1)"></iframe><iframe src="http://127.0.0.1/api"></iframe><video></video>'
    );
    expect(div.innerHTML).toBe('');
  });
});

describe('懒加载图片', () => {
  it('data-src 与 data-original 换进 src 后再按规则检查', () => {
    const div = dom(
      '<img src="data:image/gif;base64,R0lG" data-src="https://img.example/real.png">' +
        '<img data-original="http://127.0.0.1/api/hit">'
    );
    const imgs = div.querySelectorAll('img');
    expect(imgs[0].getAttribute('src')).toBe('https://img.example/real.png');
    expect(imgs[1].getAttribute('src')).toBeNull();
  });
});

describe('safeImageUrl', () => {
  it('缩略图地址按同一条规则过滤', () => {
    expect(safeImageUrl('https://img.example/a.png')).toBe('https://img.example/a.png');
    expect(safeImageUrl('data:image/png;base64,AAAA')).toBe('data:image/png;base64,AAAA');
    expect(safeImageUrl('http://127.0.0.1:1235/api/version')).toBe('');
    expect(safeImageUrl('/api/version')).toBe('');
    expect(safeImageUrl('javascript:alert(1)')).toBe('');
    expect(safeImageUrl('')).toBe('');
  });
});
