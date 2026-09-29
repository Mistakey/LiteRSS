import { describe, expect, it } from 'vitest';
import { applyBionic, fixationLength } from './bionic';

function render(html: string) {
  const root = document.createElement('div');
  root.innerHTML = html;
  applyBionic(root);
  return root;
}

const heads = (root: HTMLElement) =>
  [...root.querySelectorAll('.bionic-fix')].map((b) => b.textContent);

describe('fixationLength', () => {
  it('与 text-vide fixation 1 档的查表一致', () => {
    // text-vide getFixationLength(word, 1)，词长 1–14
    const expected = [0, 1, 2, 3, 3, 4, 5, 6, 7, 8, 9, 10, 10, 11];
    expect(expected.map((_, i) => fixationLength(i + 1))).toEqual(expected);
  });

  it('越过表尾时按表长递减', () => {
    expect(fixationLength(48)).toBe(40);
    expect(fixationLength(49)).toBe(40);
    expect(fixationLength(60)).toBe(51);
  });
});

describe('applyBionic', () => {
  it('只加粗每个词的开头', () => {
    const root = render('<p>a the most software</p>');
    expect(heads(root)).toEqual(['th', 'mos', 'softwa']);
    expect(root.textContent).toBe('a the most software');
    expect(root.querySelector('p')!.innerHTML).toBe(
      'a <span class="bionic-fix">th</span>e <span class="bionic-fix">mos</span>t ' +
        '<span class="bionic-fix">softwa</span>re'
    );
  });

  it('词由字母与数字组成、至少含一个字母；纯数字不动', () => {
    const root = render('<p>2024 年 GPT4o, x86-64 café</p>');
    expect(heads(root)).toEqual(['GPT', 'x8', 'caf']);
    expect(root.textContent).toBe('2024 年 GPT4o, x86-64 café');
  });

  it('中文不动', () => {
    const html = '<p>这是一段中文，没有英文。</p>';
    expect(render(html).innerHTML).toBe(html);
  });

  it('中英混排只动英文', () => {
    const root = render('<p>用 Vue 写界面</p>');
    expect(heads(root)).toEqual(['Vu']);
    expect(root.textContent).toBe('用 Vue 写界面');
  });

  it('跳过代码、公式、小标题、已加粗的文字与对照译文', () => {
    const html =
      '<pre><code>const value = 1</code></pre>' +
      '<p>inline <code>code here</code></p>' +
      '<h2>Heading words</h2><h6>Small heading</h6>' +
      '<p><b>bold words</b> <strong>strong words</strong></p>' +
      '<p><math><mi>alpha</mi></math> <span class="katex">katex text</span></p>' +
      '<p><span class="katex-inline"><span class="katex-error">\\frac{a}{b} broken</span></span></p>' +
      '<p>Source<span class="literss-tr">translated words</span></p>';
    const root = render(html);
    expect(heads(root)).toEqual(['inli', 'Sour']);
    expect(root.querySelector('pre')!.outerHTML).toBe('<pre><code>const value = 1</code></pre>');
    expect(root.querySelector('h2')!.innerHTML).toBe('Heading words');
    expect(root.querySelector('b')!.innerHTML).toBe('bold words');
    expect(root.querySelector('.katex')!.innerHTML).toBe('katex text');
    expect(root.querySelector('.literss-tr')!.innerHTML).toBe('translated words');
  });

  it('只看根以内的祖先', () => {
    const outer = document.createElement('h1');
    const root = document.createElement('div');
    root.innerHTML = '<p>words</p>';
    outer.append(root);
    applyBionic(root);
    expect(heads(root)).toEqual(['wor']);
  });

  it('链接里的词照样加粗，链接不变', () => {
    const root = render('<p><a href="https://x.example/">read more</a></p>');
    expect(heads(root)).toEqual(['rea', 'mor']);
    expect(root.querySelector('a')!.getAttribute('href')).toBe('https://x.example/');
  });
});
