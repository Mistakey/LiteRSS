import { describe, expect, it } from 'vitest';
import { interleave, textBlocks } from './bilingual';

const ARTICLE =
  '<h2>A  heading</h2>' +
  '<p>First <a href="https://x.example/">linked</a> paragraph with <code>inline()</code> code.</p>' +
  '<pre><code>const p = "<p>not a block</p>";</code></pre>' +
  '<ul><li>Item one</li><li>Item two<ul><li>Nested item</li></ul></li></ul>' +
  '<blockquote><p>Quoted words.</p></blockquote>' +
  '<figure><img src="https://img.example/1.png"><figcaption>A caption</figcaption></figure>' +
  '<table><tr><td><p>Cell text</p></td></tr></table>' +
  '<p>* * *</p>' +
  '<math><mi>x</mi></math>';

describe('textBlocks', () => {
  it('只取段落、列表项、小标题、引用、图注，代码块、表格、公式与图片不翻', () => {
    expect(textBlocks(ARTICLE)).toEqual([
      'A heading',
      'First linked paragraph with inline() code.',
      'Item one',
      'Item two',
      'Nested item',
      'Quoted words.',
      'A caption',
    ]);
  });

  it('没有文字的正文没有块', () => {
    expect(textBlocks('<img src="https://img.example/1.png"><pre>code</pre>')).toEqual([]);
  });
});

describe('interleave', () => {
  const translate = (html: string) =>
    interleave(
      html,
      textBlocks(html).map((t) => `译：${t}`)
    );

  it('每块原文下紧跟它的译文', () => {
    const root = document.createElement('div');
    root.innerHTML = translate(ARTICLE);
    const trs = [...root.querySelectorAll('.literss-tr')];
    expect(trs.map((t) => t.textContent)).toEqual(textBlocks(ARTICLE).map((t) => `译：${t}`));
    for (const tr of trs) expect(tr.getAttribute('lang')).toBe('zh-CN');

    const p = root.querySelector('p')!;
    expect(p.lastElementChild!.textContent).toBe('译：First linked paragraph with inline() code.');
    // 带子列表的列表项：译文紧跟自己的文字，在子列表之前。
    const parent = root.querySelectorAll('li')[1];
    expect([...parent.children].map((c) => c.tagName)).toEqual(['SPAN', 'UL']);
    // 代码块与表格原样。
    expect(root.querySelector('pre')!.querySelector('.literss-tr')).toBeNull();
    expect(root.querySelector('table')!.querySelector('.literss-tr')).toBeNull();
    expect(root.querySelector('img')!.getAttribute('src')).toBe('https://img.example/1.png');
  });

  it('译文按纯文本写入，不当 HTML 解析', () => {
    const html = interleave('<p>Hi</p>', ['<img src=x onerror=alert(1)>']);
    const root = document.createElement('div');
    root.innerHTML = html;
    expect(root.querySelector('img')).toBeNull();
    expect(root.querySelector('.literss-tr')!.textContent).toBe('<img src=x onerror=alert(1)>');
  });

  it('块数对不上时原样返回', () => {
    expect(interleave('<p>One</p><p>Two</p>', ['一'])).toBe('<p>One</p><p>Two</p>');
  });
});
