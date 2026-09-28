import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FakeBackend, feed, settle } from '../test/fakeBackend';
import { useDetail } from './detail';
import { useReader } from './reader';

const LONG = `<p>${'正文'.repeat(400)}</p>`;
const SHORT = '<p>A teaser.</p>';
const MEDIUM = `<p>${'x'.repeat(350)}</p>`;

let be: FakeBackend;

beforeEach(() => {
  setActivePinia(createPinia());
  be = new FakeBackend();
  be.tree = { categories: [], feeds: [feed('feed/1', '源')] };
  be.add(
    { id: 3, stream: 'feed/1', title: '完整', translated: '完整', content: LONG },
    { id: 2, stream: 'feed/1', title: 'Teaser', translated: '摘录', content: SHORT },
    { id: 1, stream: 'feed/1', title: 'Medium', translated: '中等', content: MEDIUM }
  );
  be.install();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function opened(id: number) {
  const reader = useReader();
  await reader.init();
  const detail = useDetail();
  await reader.open(id);
  await settle();
  return detail;
}

describe('正文与自动抓全文', () => {
  it('完整正文不抓全文', async () => {
    const detail = await opened(3);
    expect(detail.body).toBe(LONG);
    expect(detail.fetchStatus).toBe('none');
    expect(be.callsTo('POST', '/api/articles/3/fulltext')).toHaveLength(0);
  });

  it('截断正文点开即抓全文，成功后替换显示', async () => {
    be.fullTexts[2] = { outcome: 'success', content: '<p>全文</p>', message: '' };
    const detail = await opened(2);
    expect(be.callsTo('POST', '/api/articles/2/fulltext')).toHaveLength(1);
    expect(detail.fetchStatus).toBe('ok');
    expect(detail.body).toBe('<p>全文</p>');
  });

  it('抓取失败保留 RSS 正文并给中文原因；太短时不能摘要', async () => {
    be.fullTexts[2] = { outcome: 'blocked', content: '', message: '站点拒绝了这次抓取。' };
    const detail = await opened(2);
    expect(detail.fetchStatus).toBe('failed');
    expect(detail.fetchMessage).toBe('站点拒绝了这次抓取。');
    expect(detail.body).toBe(SHORT);
    expect(detail.canSummarize).toBe(false);
    await detail.summarize();
    expect(be.callsTo('POST', '/api/articles/2/summary')).toHaveLength(0);
  });

  it('抓取失败但 RSS 正文够长仍可摘要', async () => {
    be.fullTexts[1] = { outcome: 'no_content', content: '', message: '这一页没有可提取的正文。' };
    const detail = await opened(1);
    expect(detail.fetchStatus).toBe('failed');
    expect(detail.canSummarize).toBe(true);
  });

  it('换文章后旧的抓取结果不覆盖新文章', async () => {
    be.fullTexts[2] = { outcome: 'success', content: '<p>旧全文</p>', message: '' };
    const reader = useReader();
    await reader.init();
    const detail = useDetail();
    void reader.open(2);
    void reader.open(3);
    await settle();
    expect(detail.id).toBe(3);
    expect(detail.body).toBe(LONG);
    expect(detail.fetchStatus).toBe('none');
  });
});

describe('摘要', () => {
  it('点摘要才生成，带依据说明', async () => {
    be.summaries[3] = { html: '<ul><li>要点</li></ul>', note: '摘要基于 RSS 正文。' };
    const detail = await opened(3);
    expect(be.callsTo('POST', '/api/articles/3/summary')).toHaveLength(0);
    await detail.summarize();
    expect(detail.summaryHtml).toBe('<ul><li>要点</li></ul>');
    expect(detail.summaryNote).toBe('摘要基于 RSS 正文。');
    await detail.summarize();
    expect(be.callsTo('POST', '/api/articles/3/summary')).toHaveLength(1);
  });

  it('没生成时显示原因，可以再点', async () => {
    const detail = await opened(3);
    await detail.summarize();
    expect(detail.summaryHtml).toBe('');
    expect(detail.summaryNote).toBe('还没有配置摘要模型，请在设置里填写。');
    await detail.summarize();
    expect(be.callsTo('POST', '/api/articles/3/summary')).toHaveLength(2);
  });
});
