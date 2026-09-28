import { describe, expect, it } from 'vitest';
import { isTruncated, longEnoughToSummarize, visibleLength } from './article';

const long = (n: number) => `<p>${'字'.repeat(n)}</p>`;

describe('visibleLength', () => {
  it('数非空白字符，不数标签与脚本', () => {
    expect(visibleLength('<p>a b\n c</p><script>xxxx</script><style>yyyy</style>')).toBe(3);
    expect(visibleLength('<p>中文 &amp; 英文</p>')).toBe(5);
  });
});

describe('isTruncated', () => {
  it('空正文与很短的正文算截断', () => {
    expect(isTruncated('')).toBe(true);
    expect(isTruncated('<p>A short teaser about the article.</p>')).toBe(true);
  });

  it('够长且没有截断标记的不算', () => {
    expect(isTruncated(long(600))).toBe(false);
  });

  it.each([
    '…',
    '...',
    '[…]',
    '[...]',
    'Read more',
    'Continue reading →',
    '阅读全文',
    '查看全文>>',
  ])('以「%s」结尾的算截断', (tail) => {
    expect(isTruncated(`${long(600)}<p>${tail}</p>`)).toBe(true);
  });

  it('带题图的短摘录也算截断', () => {
    expect(isTruncated('<img src="https://img.example/a.png"><p>First paragraph only.</p>')).toBe(
      true
    );
  });
});

describe('longEnoughToSummarize', () => {
  it('与后端 300 字门槛一致', () => {
    expect(longEnoughToSummarize(long(299))).toBe(false);
    expect(longEnoughToSummarize(long(300))).toBe(true);
  });
});
