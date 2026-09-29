import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '../App.vue';
import { FakeBackend, feed, settle } from '../test/fakeBackend';
import { icons, type IconName } from './icons';

const LONG = `<p>${'正文'.repeat(400)}</p>`;

type Wrapper = Awaited<ReturnType<typeof openRow>>;

/** 按钮里渲染的是哪个图标：按第一个图形的属性对照图标表。 */
function iconOf(btn: ReturnType<Wrapper['find']>): IconName {
  const first = btn.find('svg > *').element;
  const name = (Object.keys(icons) as IconName[]).find((n) => {
    const [tag, attrs] = icons[n][0];
    return (
      first.tagName === tag &&
      Object.entries(attrs).every(([k, v]) => first.getAttribute(k) === v) &&
      btn.findAll('svg > *').length === icons[n].length
    );
  });
  if (!name) throw new Error('unknown icon');
  return name;
}

const bar = (w: Wrapper) => w.findAll('.float-bar .btn');
const button = (w: Wrapper, key: string) => w.find(`.float-bar .btn.${key}`);
const label = (w: Wrapper, key: string) => button(w, key).attributes('aria-label');

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
    expect(
      bar(w).map((b) => [b.attributes('aria-label'), b.attributes('title'), iconOf(b)])
    ).toEqual([
      ['生成摘要', '生成摘要', 'spark'],
      ['翻译：中英段落对照', '翻译：中英段落对照', 'translate'],
      ['Bionic Reading：加粗英文词首', 'Bionic Reading：加粗英文词首', 'bionicOff'],
      ['抓取全文', '抓取全文', 'cloud'],
      ['在浏览器打开', '在浏览器打开', 'ext'],
      ['已读，点击标为未读', '已读，点击标为未读', 'mailOpen'],
    ]);
    expect(bar(w).map((b) => b.text())).toEqual(['', '', '', '', '', '']);
    expect(bar(w).map((b) => b.attributes('aria-pressed'))).toEqual([
      undefined,
      'false',
      'false',
      undefined,
      undefined,
      undefined,
    ]);
    expect(w.find('.float-bar .div + .btn').classes()).toContain('read');
    expect(w.find('.notice').exists()).toBe(false);
    w.unmount();
  });

  it('正文链接交给系统浏览器；已读按钮图标表示当前状态，点击切换', async () => {
    const w = await openRow(3);
    await w.find('.article-body a').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/browser/open')[0].body).toEqual({
      url: 'https://site.example/p',
    });
    await button(w, 'read').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/articles/3/read').at(-1)!.body).toEqual({ read: false });
    expect(label(w, 'read')).toBe('未读，点击标为已读');
    expect(iconOf(button(w, 'read'))).toBe('mail');
    w.unmount();
  });

  it('打开不抓全文；RSS 正文太短时提示可以先抓全文，摘要不可用', async () => {
    const w = await openRow(2);
    expect(be.callsTo('POST', '/api/articles/2/fulltext')).toHaveLength(0);
    const notice = w.find('.notice.info');
    expect(notice.text()).toContain('可以先抓取全文');
    expect(notice.find('.btn.primary').text()).toBe('抓取全文');
    expect(button(w, 'summary').attributes('disabled')).toBeDefined();
    expect(label(w, 'summary')).toBe('正文太短，可以先抓取全文');
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
    expect(button(w, 'fetch').exists()).toBe(false);
    expect(label(w, 'summary')).toBe('正文太短，无法摘要');
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
    await button(w, 'fetch').trigger('click');
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
    await button(w, 'summary').trigger('click');
    await settle();
    expect(w.find('.summary li').text()).toBe('RSS 要点');
    expect(w.find('.summary .note').text()).toBe('摘要基于 RSS 正文。');

    be.fullTexts[1] = { outcome: 'success', content: LONG, message: '' };
    be.summaries[1] = { html: '<ul><li>全文要点</li></ul>', note: '' };
    await button(w, 'fetch').trigger('click');
    await settle();
    expect(w.find('.summary li').text()).toBe('全文要点');
    expect(w.find('.summary .note').exists()).toBe(false);
    expect(label(w, 'summary')).toBe('已生成摘要');
    expect(iconOf(button(w, 'summary'))).toBe('sparkCheck');
    expect(button(w, 'summary').attributes('disabled')).toBeDefined();
    w.unmount();
  });
});

describe('浮动条的进行中状态', () => {
  it('摘要生成中、抓取全文中换成转圈并不可点', async () => {
    let finishSummary!: (s: { html: string; note: string }) => void;
    be.summaries[3] = new Promise((r) => (finishSummary = r));
    let finishFetch!: (f: { outcome: string; content: string; message: string }) => void;
    be.fullTexts[3] = new Promise((r) => (finishFetch = r));
    const w = await openRow(3);

    await button(w, 'summary').trigger('click');
    await settle();
    expect(iconOf(button(w, 'summary'))).toBe('refresh');
    expect(button(w, 'summary').find('svg').classes()).toContain('spin');
    expect(label(w, 'summary')).toBe('摘要生成中…');
    expect(button(w, 'summary').attributes('disabled')).toBeDefined();
    finishSummary({ html: '<ul><li>要点</li></ul>', note: '' });
    await settle();

    await button(w, 'fetch').trigger('click');
    await settle();
    expect(iconOf(button(w, 'fetch'))).toBe('refresh');
    expect(label(w, 'fetch')).toBe('正在抓取全文…');
    expect(button(w, 'fetch').attributes('disabled')).toBeDefined();
    expect(label(w, 'translate')).toBe('全文抓取中，稍后可以翻译');
    expect(button(w, 'translate').attributes('disabled')).toBeDefined();
    finishFetch({ outcome: 'blocked', content: '', message: '站点拒绝了这次抓取。' });
    await settle();
    expect(iconOf(button(w, 'fetch'))).toBe('cloud');
    w.unmount();
  });
});

describe('Bionic Reading', () => {
  const heads = (w: Wrapper) => w.findAll('.article-body .bionic-fix').map((b) => b.text());
  /** 等增强与加粗落定，再多等一会儿，确认没有第二次加粗。 */
  async function settledHeads(w: Wrapper, expected: string[]) {
    await vi.waitFor(() => expect(heads(w)).toEqual(expected));
    await new Promise((r) => setTimeout(r, 100));
    expect(heads(w)).toEqual(expected);
  }

  beforeEach(() => {
    be.byId(3)!.content =
      '<p>Reading software</p><pre><code>const value</code></pre><p>中文段落</p>';
  });

  it('默认关；打开后正文重建、词首只加粗一次，并记进设置；再点关闭恢复原文', async () => {
    const w = await openRow(3);
    expect(heads(w)).toEqual([]);
    expect(iconOf(button(w, 'bionic'))).toBe('bionicOff');

    await button(w, 'bionic').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/settings/update').at(-1)!.body).toEqual({
      bionic_reading: true,
    });
    // 正文含代码：先等 highlight.js 增强，再加粗
    await vi.waitFor(() => expect(heads(w)).toEqual(['Readi', 'softwa']));
    expect(w.find('.article-body pre').text()).toBe('const value');
    expect(w.find('.article-body').text()).toBe('Reading softwareconst value中文段落');
    expect(button(w, 'bionic').attributes('aria-pressed')).toBe('true');
    expect(button(w, 'bionic').classes()).toContain('active');
    expect(label(w, 'bionic')).toBe('Bionic Reading 已开，点击关闭');
    expect(iconOf(button(w, 'bionic'))).toBe('bionic');

    await button(w, 'bionic').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/settings/update').at(-1)!.body).toEqual({
      bionic_reading: false,
    });
    expect(heads(w)).toEqual([]);
    expect(w.find('.article-body p').html()).toBe('<p>Reading software</p>');
    w.unmount();
  });

  it('设置里记着开：打开文章即加粗；换文章与对照译文时仍只加粗一次，译文不动', async () => {
    be.settings.bionic_reading = true;
    be.translator = (_id, blocks) => ({ blocks: blocks.map(() => 'Translated text'), message: '' });
    const w = await openRow(3);
    expect(heads(w)).toEqual(['Readi', 'softwa']);

    await button(w, 'translate').trigger('click');
    await settle();
    expect(heads(w)).toEqual(['Readi', 'softwa']);
    expect(w.findAll('.article-body .literss-tr').map((t) => t.html())).toEqual([
      '<span class="literss-tr" lang="zh-CN">Translated text</span>',
      '<span class="literss-tr" lang="zh-CN">Translated text</span>',
    ]);

    await w
      .findAll('.row')
      .find((r) => r.text().includes('中等'))!
      .trigger('click');
    await settle();
    expect(w.findAll('.article-body .bionic-fix')).toHaveLength(1);
    w.unmount();
  });
});

describe('全文翻译', () => {
  const translateBtn = (w: Wrapper) => button(w, 'translate');
  const translateState = (w: Wrapper) => [label(w, 'translate'), iconOf(translateBtn(w))];
  const translations = (w: Wrapper) => w.findAll('.article-body .literss-tr').map((t) => t.text());

  beforeEach(() => {
    be.byId(3)!.content =
      '<p>First paragraph.</p><pre><code>code()</code></pre><ul><li>An item</li></ul>';
  });

  it('点三次依次为对照 → 原文 → 对照，第三次不再请求', async () => {
    be.translator = (_id, blocks) => ({ blocks: blocks.map((b) => '译：' + b), message: '' });
    const w = await openRow(3);
    expect(translateState(w)).toEqual(['翻译：中英段落对照', 'translate']);

    await translateBtn(w).trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/articles/3/translation')[0].body).toEqual({
      blocks: ['First paragraph.', 'An item'],
    });
    expect(translations(w)).toEqual(['译：First paragraph.', '译：An item']);
    expect(w.find('.article-body p .literss-tr').text()).toBe('译：First paragraph.');
    expect(w.find('.article-body pre .literss-tr').exists()).toBe(false);
    expect(translateState(w)).toEqual(['正在对照，点击回到原文', 'bilingual']);
    expect(translateBtn(w).attributes('aria-pressed')).toBe('true');
    expect(translateBtn(w).classes()).toContain('active');

    await translateBtn(w).trigger('click');
    await settle();
    expect(translations(w)).toEqual([]);
    expect(w.find('.article-body').text()).toContain('First paragraph.');
    expect(translateState(w)).toEqual(['翻译：中英段落对照', 'translate']);

    await translateBtn(w).trigger('click');
    await settle();
    expect(translations(w)).toEqual(['译：First paragraph.', '译：An item']);
    expect(be.callsTo('POST', '/api/articles/3/translation')).toHaveLength(1);
    w.unmount();
  });

  it('标题判定为中文的文章没有「翻译」', async () => {
    const w = await openRow(2);
    expect(translateBtn(w).exists()).toBe(false);
    w.unmount();
  });

  it('翻译中显示「翻译中…」，失败显示中文原因并停在原文', async () => {
    let finish!: (t: { blocks: string[]; message: string }) => void;
    be.translator = () => new Promise((r) => (finish = r));
    const w = await openRow(3);
    await translateBtn(w).trigger('click');
    await settle();
    expect(translateState(w)).toEqual(['翻译中…', 'refresh']);
    expect(translateBtn(w).attributes('aria-pressed')).toBe('false');
    expect(translateBtn(w).attributes('disabled')).toBeDefined();

    finish({ blocks: [], message: '全文翻译失败，请检查设置里的大模型，或稍后再试。' });
    await settle();
    expect(w.find('.notice.translate-failed').text()).toBe(
      '全文翻译失败，请检查设置里的大模型，或稍后再试。'
    );
    expect(translations(w)).toEqual([]);
    expect(translateState(w)).toEqual(['翻译：中英段落对照', 'translate']);
    w.unmount();
  });

  it('抓到全文后旧译文作废，回到原文，再翻按全文请求', async () => {
    be.translator = (_id, blocks) => ({ blocks: blocks.map((b) => '译：' + b), message: '' });
    be.fullTexts[3] = { outcome: 'success', content: '<p>The full text.</p>', message: '' };
    const w = await openRow(3);
    await translateBtn(w).trigger('click');
    await settle();
    expect(translations(w)).toHaveLength(2);

    await button(w, 'fetch').trigger('click');
    await settle();
    expect(w.find('.article-body').text()).toBe('The full text.');
    expect(translateState(w)).toEqual(['翻译：中英段落对照', 'translate']);

    await translateBtn(w).trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/articles/3/translation').at(-1)!.body).toEqual({
      blocks: ['The full text.'],
    });
    expect(translations(w)).toEqual(['译：The full text.']);
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
