import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '../App.vue';
import { FakeBackend, feed, settle } from '../test/fakeBackend';

const LONG = `<p>${'正文'.repeat(400)}</p>`;

let be: FakeBackend;

beforeEach(() => {
  setActivePinia(createPinia());
  be = new FakeBackend();
  be.tree = { categories: [], feeds: [feed('feed/1', '少数派')] };
  be.add(
    {
      id: 3,
      stream: 'feed/1',
      title: 'English title',
      translated: '英文译文',
      content:
        LONG +
        '<p><a href="https://site.example/p">链接</a></p>' +
        '<img src="https://img.example/1.png"><img src="https://img.example/2.png">' +
        '<script>window.hacked = 1</script><img src="/api/hit">',
    },
    { id: 2, stream: 'feed/1', title: '短文', translated: '短文', content: '<p>摘录…</p>' },
    {
      id: 1,
      stream: 'feed/1',
      title: '中等',
      translated: '中等',
      content: `<p>${'x'.repeat(350)}…</p>`,
    }
  );
  be.install();
});

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.innerHTML = '';
});

async function openRow(id: number) {
  const w = mount(App, { attachTo: document.body });
  await settle();
  const row = w.findAll('.row').find((r) => r.text().includes(be.byId(id)!.translated!))!;
  await row.trigger('click');
  await settle();
  return w;
}

describe('详情', () => {
  it('标题块显示译文与原标题，正文经清洗，浮动条的动作', async () => {
    const w = await openRow(3);
    expect(w.find('.reader h1').text()).toBe('英文译文');
    expect(w.find('.reader .orig').text()).toBe('English title');
    const body = w.find('.article-body');
    expect(body.find('script').exists()).toBe(false);
    expect(body.findAll('img[src]')).toHaveLength(2);
    expect(w.findAll('.float-bar .btn').map((b) => b.text())).toEqual([
      '摘要',
      '抓取全文',
      '在浏览器打开',
      '标为未读',
    ]);
    expect(w.find('.notice').exists()).toBe(false);
    w.unmount();
  });

  it('正文链接交给系统浏览器，「标为未读」改回未读', async () => {
    const w = await openRow(3);
    await w.find('.article-body a').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/browser/open')[0].body).toEqual({
      url: 'https://site.example/p',
    });
    await w.find('.float-bar .btn.ghost').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/articles/3/read').at(-1)!.body).toEqual({ read: false });
    expect(w.find('.float-bar .btn.ghost').text()).toBe('标为已读');
    w.unmount();
  });

  it('打开不抓全文；RSS 正文太短时提示可以先抓全文，摘要不可用', async () => {
    const w = await openRow(2);
    expect(be.callsTo('POST', '/api/articles/2/fulltext')).toHaveLength(0);
    const notice = w.find('.notice.info');
    expect(notice.text()).toContain('可以先抓取全文');
    expect(notice.find('.btn.primary').text()).toBe('抓取全文');
    expect(w.find('.float-bar .btn').attributes('disabled')).toBeDefined();
    expect(w.find('.article-body').text()).toBe('摘录…');
    w.unmount();
  });

  it('点「抓取全文」成功：全文替换显示，按钮与提示消失', async () => {
    be.fullTexts[2] = { outcome: 'success', content: '<p>抓到的全文</p>', message: '' };
    const w = await openRow(2);
    await w.find('.notice .btn.primary').trigger('click');
    await settle();
    expect(w.find('.article-body').text()).toBe('抓到的全文');
    expect(w.find('.notice').exists()).toBe(false);
    expect(w.findAll('.float-bar .btn').map((b) => b.text())).not.toContain('抓取全文');
    w.unmount();
  });

  it('已缓存全文的文章打开即显示全文', async () => {
    be.cachedFullTexts[2] = '<p>缓存的全文</p>';
    const w = await openRow(2);
    expect(w.find('.article-body').text()).toBe('缓存的全文');
    expect(w.find('.notice').exists()).toBe(false);
    w.unmount();
  });

  it('抓取失败：保留 RSS 正文，显示原因与在浏览器打开', async () => {
    be.fullTexts[2] = { outcome: 'blocked', content: '', message: '站点拒绝了这次抓取。' };
    const w = await openRow(2);
    await w.findAll('.float-bar .btn')[1].trigger('click');
    await settle();
    const notice = w.find('.notice.warn');
    expect(notice.text()).toContain('站点拒绝了这次抓取。');
    expect(notice.text()).toContain('无法生成摘要');
    expect(notice.find('.btn.primary').text()).toBe('在浏览器打开');
    expect(w.find('.article-body').text()).toBe('摘录…');
    w.unmount();
  });

  it('摘要后抓到全文：摘要按全文重新生成', async () => {
    be.summaries[1] = { html: '<ul><li>RSS 要点</li></ul>', note: '摘要基于 RSS 正文。' };
    const w = await openRow(1);
    await w.find('.float-bar .btn').trigger('click');
    await settle();
    expect(w.find('.summary li').text()).toBe('RSS 要点');
    expect(w.find('.summary .note').text()).toBe('摘要基于 RSS 正文。');

    be.fullTexts[1] = { outcome: 'success', content: LONG, message: '' };
    be.summaries[1] = { html: '<ul><li>全文要点</li></ul>', note: '' };
    await w.findAll('.float-bar .btn')[1].trigger('click');
    await settle();
    expect(w.find('.summary li').text()).toBe('全文要点');
    expect(w.find('.summary .note').exists()).toBe(false);
    expect(w.find('.float-bar .btn').text()).toBe('已摘要');
    w.unmount();
  });
});

describe('图片查看器', () => {
  it('点图打开，多图左右切换，Esc 关闭', async () => {
    const w = await openRow(3);
    await w.findAll('.article-body img')[1].trigger('click');
    const viewer = () => document.querySelector('.viewer');
    expect(viewer()).not.toBeNull();
    expect(viewer()!.querySelector('.count')!.textContent).toBe('2 / 2');
    expect(viewer()!.querySelector('img')!.getAttribute('src')).toBe('https://img.example/2.png');

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }));
    await settle();
    expect(viewer()!.querySelector('.count')!.textContent).toBe('1 / 2');

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await settle();
    expect(viewer()).toBeNull();
    w.unmount();
  });

  it('点背景关闭，点图片不关闭', async () => {
    const w = await openRow(3);
    await w.find('.article-body img').trigger('click');
    const viewer = () => document.querySelector<HTMLElement>('.viewer');
    viewer()!.querySelector('img')!.click();
    await settle();
    expect(viewer()).not.toBeNull();
    viewer()!.click();
    await settle();
    expect(viewer()).toBeNull();
    w.unmount();
  });
});
