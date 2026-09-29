import { describe, expect, it } from 'vitest';
import { longEnoughToSummarize, visibleLength } from './article';

const long = (n: number) => `<p>${'字'.repeat(n)}</p>`;

describe('visibleLength', () => {
  it('数非空白字符，不数标签与脚本', () => {
    expect(visibleLength('<p>a b\n c</p><script>xxxx</script><style>yyyy</style>')).toBe(3);
    expect(visibleLength('<p>中文 &amp; 英文</p>')).toBe(5);
  });
});

describe('longEnoughToSummarize', () => {
  it('与后端 300 字门槛一致', () => {
    expect(longEnoughToSummarize(long(299))).toBe(false);
    expect(longEnoughToSummarize(long(300))).toBe(true);
  });
});
