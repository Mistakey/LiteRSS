import { describe, expect, it } from 'vitest';
import { highlightCode, renderMath } from './enhance';

function root(html: string) {
  const div = document.createElement('div');
  div.innerHTML = html;
  return div;
}

describe('renderMath', () => {
  it('行内与独立公式', () => {
    const div = root('<p>行内 $E = mc^2$ 与 $$\\int x dx$$</p>');
    renderMath(div);
    expect(div.querySelector('.katex-inline .katex')).not.toBeNull();
    expect(div.querySelector('.katex-display .katex')).not.toBeNull();
  });

  it('不把金额当公式，不动代码里的 $', () => {
    const html = '<p>价格 $5 和 $10 之间</p><pre><code>echo $HOME$</code></pre>';
    const div = root(html);
    renderMath(div);
    expect(div.innerHTML).toBe(html);
  });

  it('读 FreshRSS 改名后的 math 类', () => {
    const div = root('<span data-sanitized-class="math">x^2</span>');
    renderMath(div);
    expect(div.querySelector('.katex-inline .katex')).not.toBeNull();
  });
});

describe('highlightCode', () => {
  it('语言取自 data-sanitized-class', () => {
    const div = root('<pre><code data-sanitized-class="language-go">package main</code></pre>');
    highlightCode(div);
    const code = div.querySelector('code')!;
    expect(code.classList.contains('hljs')).toBe(true);
    expect(code.querySelector('.hljs-keyword')!.textContent).toBe('package');
  });

  it('没有 code 的 pre 也高亮，原文不变', () => {
    const div = root('<pre>SELECT 1;</pre>');
    highlightCode(div);
    expect(div.querySelector('pre')!.classList.contains('hljs')).toBe(true);
    expect(div.textContent).toBe('SELECT 1;');
  });
});
