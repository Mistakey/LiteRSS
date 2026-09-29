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

describe('正文与抓全文', () => {
  it('打开只显示 RSS 正文，不自动抓全文', async () => {
    const detail = await opened(2);
    expect(detail.body).toBe(SHORT);
    expect(detail.fetchStatus).toBe('none');
    expect(be.callsTo('POST', '/api/articles/2/fulltext')).toHaveLength(0);
  });

  it('已缓存全文的直接显示全文', async () => {
    be.cachedFullTexts[2] = '<p>缓存的全文</p>';
    const detail = await opened(2);
    expect(detail.fetchStatus).toBe('ok');
    expect(detail.body).toBe('<p>缓存的全文</p>');
    expect(be.callsTo('POST', '/api/articles/2/fulltext')).toHaveLength(0);
  });

  it('点「抓取全文」成功后替换显示', async () => {
    be.fullTexts[2] = { outcome: 'success', content: '<p>全文</p>', message: '' };
    const detail = await opened(2);
    await detail.fetchFullText();
    expect(be.callsTo('POST', '/api/articles/2/fulltext')).toHaveLength(1);
    expect(detail.fetchStatus).toBe('ok');
    expect(detail.body).toBe('<p>全文</p>');
  });

  it('抓取失败保留 RSS 正文并给中文原因', async () => {
    be.fullTexts[1] = { outcome: 'blocked', content: '', message: '站点拒绝了这次抓取。' };
    const detail = await opened(1);
    await detail.fetchFullText();
    expect(detail.fetchStatus).toBe('failed');
    expect(detail.fetchMessage).toBe('站点拒绝了这次抓取。');
    expect(detail.body).toBe(MEDIUM);
    expect(detail.canSummarize).toBe(true);
  });

  it('换文章后旧的抓取结果不覆盖新文章', async () => {
    be.fullTexts[2] = { outcome: 'success', content: '<p>旧全文</p>', message: '' };
    const reader = useReader();
    await reader.init();
    const detail = useDetail();
    await reader.open(2);
    await settle();
    const pending = detail.fetchFullText();
    await reader.open(3);
    await pending;
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
    expect(detail.summaryNote).toBe('还没有配置大模型，请在设置里填写。');
    await detail.summarize();
    expect(be.callsTo('POST', '/api/articles/3/summary')).toHaveLength(2);
  });

  it('RSS 正文太短时不能摘要，提示可以先抓全文', async () => {
    const detail = await opened(2);
    expect(detail.canSummarize).toBe(false);
    expect(detail.suggestFullText).toBe(true);
    await detail.summarize();
    expect(be.callsTo('POST', '/api/articles/2/summary')).toHaveLength(0);

    be.fullTexts[2] = { outcome: 'success', content: LONG, message: '' };
    await detail.fetchFullText();
    expect(detail.canSummarize).toBe(true);
    expect(detail.suggestFullText).toBe(false);
  });

  it('抓到全文后，已有的摘要按全文重新生成', async () => {
    be.summaries[1] = { html: '<p>RSS 摘要</p>', note: '摘要基于 RSS 正文。' };
    const detail = await opened(1);
    await detail.summarize();
    expect(detail.summaryHtml).toBe('<p>RSS 摘要</p>');

    be.fullTexts[1] = { outcome: 'success', content: LONG, message: '' };
    be.summaries[1] = { html: '<p>全文摘要</p>', note: '' };
    await detail.fetchFullText();
    await settle();
    expect(be.callsTo('POST', '/api/articles/1/summary')).toHaveLength(2);
    expect(detail.summaryHtml).toBe('<p>全文摘要</p>');
    expect(detail.summaryNote).toBe('');
  });

  it('没有摘要时抓到全文不自动生成', async () => {
    be.fullTexts[1] = { outcome: 'success', content: LONG, message: '' };
    const detail = await opened(1);
    await detail.fetchFullText();
    await settle();
    expect(be.callsTo('POST', '/api/articles/1/summary')).toHaveLength(0);
  });
});

describe('全文翻译', () => {
  type Tr = { blocks: string[]; message: string };

  it('换文章后慢到的译文丢弃，新文章停在原文', async () => {
    let finish!: (t: Tr) => void;
    be.translator = () => new Promise<Tr>((r) => (finish = r));
    const detail = await opened(2);
    const pending = detail.toggleTranslation();
    await settle();
    expect(detail.translating).toBe(true);

    await useReader().open(1);
    await settle();
    finish({ blocks: ['译文'], message: '' });
    await pending;
    expect(detail.id).toBe(1);
    expect(detail.translation).toBeNull();
    expect(detail.bilingual).toBe(false);
    expect(detail.translating).toBe(false);
  });

  it('翻译途中抓到全文：按 RSS 正文翻的译文丢弃，回到原文', async () => {
    let finish!: (t: Tr) => void;
    be.translator = () => new Promise<Tr>((r) => (finish = r));
    be.fullTexts[2] = { outcome: 'success', content: '<p>The full text.</p>', message: '' };
    const detail = await opened(2);
    const pending = detail.toggleTranslation();
    await settle();
    await detail.fetchFullText();
    finish({ blocks: ['摘录的译文'], message: '' });
    await pending;
    expect(detail.body).toBe('<p>The full text.</p>');
    expect(detail.translation).toBeNull();
    expect(detail.bilingual).toBe(false);
    expect(detail.translating).toBe(false);
  });

  it('正文没有可翻的文字时不请求，给出原因', async () => {
    be.byId(2)!.content = '<pre>code()</pre>';
    be.translator = (_id, blocks) => ({ blocks, message: '' });
    const detail = await opened(2);
    await detail.toggleTranslation();
    expect(be.callsTo('POST', '/api/articles/2/translation')).toHaveLength(0);
    expect(detail.translateMessage).toBe('这篇文章没有可翻译的文字。');
  });
});
