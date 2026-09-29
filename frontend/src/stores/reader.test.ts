import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { READING_LIST } from '../api';
import { FakeBackend, feed, settle } from '../test/fakeBackend';
import { useReader } from './reader';
import { useSnackbar } from './snackbar';

let be: FakeBackend;

beforeEach(() => {
  setActivePinia(createPinia());
  be = new FakeBackend();
  be.tree = {
    categories: [{ id: 'user/-/label/科技', label: '科技', feeds: [feed('feed/1', '少数派')] }],
    feeds: [feed('feed/2', 'The Verge')],
  };
  be.add(
    { id: 30, stream: 'feed/1', title: '三', translated: '三' },
    { id: 20, stream: 'feed/2', title: 'Two', translated: '二' },
    { id: 10, stream: 'feed/1', title: '一', translated: '一' },
    { id: 5, stream: 'feed/1', title: '旧的已读', translated: '旧的已读', read: true }
  );
  be.install();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

async function started() {
  const reader = useReader();
  await reader.init();
  return reader;
}

describe('列表快照', () => {
  it('进入视图取快照并载入卡片，未读视图不含已读', async () => {
    const reader = await started();
    expect(reader.snapshot.ids).toEqual([30, 20, 10]);
    expect(reader.rows.map((c) => c.id)).toEqual([30, 20, 10]);
    expect(reader.counts.total).toBe(3);
  });

  it('快照在视图内稳定：点开即已读只变灰，不离开列表', async () => {
    const reader = await started();
    await reader.open(20);

    expect(be.callsTo('POST', '/api/articles/20/read')[0].body).toEqual({ read: true });
    expect(reader.snapshot.ids).toEqual([30, 20, 10]);
    expect(reader.cards.get(20)?.read).toBe(true);
    expect(reader.selectedId).toBe(20);
    expect(reader.counts.total).toBe(2);
  });

  it('批量已读后成员不变，同步完成也不替换列表', async () => {
    const reader = await started();
    await reader.markRange(20, 'above');
    await reader.afterSync();

    expect(reader.snapshot.ids).toEqual([30, 20, 10]);
    expect(reader.rows.filter((c) => c.read).map((c) => c.id)).toEqual([30, 20]);
  });

  it('切换视图或范围才重新取快照', async () => {
    const reader = await started();
    await reader.open(30);
    await reader.setView('all');
    expect(reader.snapshot.ids).toEqual([30, 20, 10, 5]);
    expect(reader.selectedId).toBeNull();

    await reader.setView('unread');
    expect(reader.snapshot.ids).toEqual([20, 10]);

    await reader.setStream('feed/2');
    expect(reader.snapshot.ids).toEqual([20]);
  });

  it('慢到的旧视图快照不覆盖新视图', async () => {
    const reader = await started();
    const first = reader.setView('all');
    const second = reader.setView('unread');
    await Promise.all([first, second]);
    expect(reader.view).toBe('unread');
    expect(reader.snapshot.ids).toEqual([30, 20, 10]);
  });
});

describe('新文章横幅', () => {
  it('同步到的新条目只进横幅，点击才载入', async () => {
    const reader = await started();
    be.add({ id: 40, stream: 'feed/2', title: 'New', translated: '新' });
    await reader.afterSync();

    expect(reader.snapshot.ids).toEqual([30, 20, 10]);
    expect(reader.rows.map((c) => c.id)).not.toContain(40);
    expect(reader.newCount).toBe(1);
    expect(reader.counts.total).toBe(4);

    await reader.loadNew();
    expect(reader.snapshot.ids).toEqual([40, 30, 20, 10]);
    expect(reader.rows[0].id).toBe(40);
    expect(reader.newCount).toBe(0);
  });

  it('只数当前范围的新条目', async () => {
    const reader = await started();
    await reader.setStream('feed/1');
    be.add({ id: 40, stream: 'feed/2', title: 'New' });
    await reader.afterSync();
    expect(reader.newCount).toBe(0);
  });

  it('同步后刷新已载入卡片的显示状态（别处读掉的变灰）', async () => {
    const reader = await started();
    be.byId(10)!.read = true;
    await reader.afterSync();
    expect(reader.cards.get(10)?.read).toBe(true);
    expect(reader.snapshot.ids).toContain(10);
  });
});

describe('批量已读与撤销', () => {
  it('此篇及以下把快照范围内的 ID 发给后端', async () => {
    const reader = await started();
    await reader.markRange(20, 'below');
    expect(be.callsTo('POST', '/api/articles/read')[0].body).toEqual({ ids: [20, 10] });
    expect(useSnackbar().current?.text).toBe('此篇及以下 2 篇已标为已读');
  });

  it('撤销调用后端令牌，并把文章恢复为未读', async () => {
    const reader = await started();
    await reader.markRange(20, 'above');
    const snackbar = useSnackbar();
    expect(snackbar.current?.token).toBe('t1');

    await snackbar.undo();
    await settle();
    expect(be.callsTo('POST', '/api/undo')[0].body).toEqual({ token: 't1' });
    expect(reader.cards.get(30)?.read).toBe(false);
    expect(reader.cards.get(20)?.read).toBe(false);
    expect(reader.counts.total).toBe(3);
    expect(snackbar.current).toBeNull();
  });

  it('当前视图全部已读以快照的 newest 为 ts', async () => {
    const reader = await started();
    be.add({ id: 99, stream: 'feed/1', title: '快照之后到的' });
    await reader.markStream(READING_LIST);
    expect(be.callsTo('POST', '/api/streams/read')[0].body).toEqual({
      stream: READING_LIST,
      ts: 30,
    });
    expect(be.byId(99)?.read).toBeFalsy();
    expect(useSnackbar().current?.text).toBe('「全部订阅」3 篇已标为已读');
    expect(reader.rows.every((c) => c.read)).toBe(true);
  });

  it('侧栏右键非当前视图时不带 ts，由后端取本地最新', async () => {
    const reader = await started();
    await reader.markStream('user/-/label/科技');
    expect(be.callsTo('POST', '/api/streams/read')[0].body).toEqual({
      stream: 'user/-/label/科技',
    });
    expect(useSnackbar().current?.text).toBe('「科技」2 篇已标为已读');
    expect(reader.cards.get(20)?.read).toBe(false);
  });

  it('没有可标的文章时提示但不给撤销', async () => {
    const reader = await started();
    await reader.markRange(20, 'above');
    await reader.markRange(20, 'above');
    expect(useSnackbar().current).toMatchObject({ text: '没有要标为已读的未读文章' });
    expect(useSnackbar().current?.token).toBeUndefined();
  });
});

describe('失败与并发', () => {
  it('操作之前发出的刷新不覆盖之后的本地改动', async () => {
    const reader = await started();
    be.byId(20)!.read = false;
    const refreshing = reader.afterSync(); // 卡片请求已发出，回来的还是 20 未读
    await reader.open(20);
    await refreshing;
    expect(reader.cards.get(20)?.read).toBe(true);
  });

  it('翻页失败不跳过这一页，下次重试', async () => {
    for (let i = 1; i <= 130; i++) be.add({ id: 1000 + i, stream: 'feed/2', title: `#${i}` });
    const reader = await started();
    const real = fetch;
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('boom', { status: 500 }))
    );
    await reader.loadMore();
    expect(reader.rows).toHaveLength(100);
    expect(reader.hasMore).toBe(true);
    vi.stubGlobal('fetch', real);
    await reader.loadMore();
    expect(reader.rows).toHaveLength(133);
  });

  it('翻页请求在途时切换视图，新视图照常载入第一页', async () => {
    for (let i = 1; i <= 130; i++) be.add({ id: 1000 + i, stream: 'feed/2', title: `#${i}` });
    const reader = await started();
    const paging = reader.loadMore();
    await reader.setView('all');
    await paging;
    expect(reader.snapshot.ids).toHaveLength(134);
    expect(reader.rows).toHaveLength(100);
  });

  it('换快照期间滚到底部不载入旧快照的页', async () => {
    for (let i = 1; i <= 130; i++) be.add({ id: 1000 + i, stream: 'feed/2', title: `#${i}` });
    const reader = await started();
    const switching = reader.setStream('feed/1');
    await reader.loadMore();
    await switching;
    expect(reader.snapshot.ids).toEqual([30, 10]);
    expect(reader.rows.map((c) => c.id)).toEqual([30, 10]);
    expect(reader.hasMore).toBe(false);
  });

  it('标已读请求失败时恢复显示并提示', async () => {
    const reader = await started();
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('boom', { status: 500 }))
    );
    await reader.open(20);
    expect(reader.cards.get(20)?.read).toBe(false);
    expect(useSnackbar().current?.text).toBe('标为已读失败，请稍后重试');
  });
});

describe('单篇已读', () => {
  it('标为未读写回后端，同 URL 的已载入卡片一起变', async () => {
    be.byId(10)!.url = 'https://same';
    be.add({ id: 11, stream: 'feed/2', title: 'dup', url: 'https://same', published: 9 });
    const reader = await started();
    await reader.setRead(10, true);
    expect(reader.cards.get(11)?.read).toBe(true);
    await reader.setRead(10, false);
    expect(be.callsTo('POST', '/api/articles/10/read')[1].body).toEqual({ read: false });
    expect(reader.cards.get(10)?.read).toBe(false);
    expect(reader.cards.get(11)?.read).toBe(false);
  });
});

describe('标题译文', () => {
  it('只为未判定的标题请求一次，拿到的译文写回卡片', async () => {
    be.add(
      { id: 50, stream: 'feed/2', title: 'Hello' },
      { id: 51, stream: 'feed/2', title: 'Fail' }
    );
    be.translations = { 50: '你好' };
    const reader = await started();
    await settle();

    const calls = be.callsTo('POST', '/api/articles/translate-titles');
    expect(calls).toHaveLength(1);
    expect(calls[0].body).toEqual({ ids: [51, 50] });
    expect(reader.cards.get(50)?.translated_title).toBe('你好');

    await reader.afterSync();
    await settle();
    expect(be.callsTo('POST', '/api/articles/translate-titles')).toHaveLength(1);
  });
});

describe('分页载入卡片', () => {
  it('按页载入快照里的卡片，loadMore 取下一页', async () => {
    for (let i = 1; i <= 130; i++)
      be.add({ id: 1000 + i, stream: 'feed/2', title: `#${i}`, translated: `#${i}` });
    const reader = await started();
    expect(reader.snapshot.ids).toHaveLength(133);
    expect(reader.rows).toHaveLength(100);
    expect(reader.hasMore).toBe(true);
    await reader.loadMore();
    expect(reader.rows).toHaveLength(133);
    expect(reader.hasMore).toBe(false);
  });
});

describe('范围名', () => {
  it('全部订阅、分类与源', async () => {
    const reader = await started();
    expect(reader.streamName(READING_LIST)).toBe('全部订阅');
    expect(reader.streamName('user/-/label/科技')).toBe('科技');
    expect(reader.streamName('feed/2')).toBe('The Verge');
  });
});

describe('侧栏树', () => {
  function withQuietFeeds() {
    be.tree = {
      categories: [
        {
          id: 'user/-/label/科技',
          label: '科技',
          feeds: [feed('feed/1', '少数派'), feed('feed/3', '读完的源')],
        },
        { id: 'user/-/label/安静', label: '安静', feeds: [feed('feed/4', '没有未读')] },
      ],
      feeds: [feed('feed/2', 'The Verge'), feed('feed/5', '也没有未读')],
    };
  }
  const ids = (reader: ReturnType<typeof useReader>) => ({
    categories: reader.sidebarTree.categories.map((c) => [c.id, c.feeds.map((f) => f.id)]),
    feeds: reader.sidebarTree.feeds.map((f) => f.id),
  });

  it('未读视图不列零未读的源，没有可显示源的分类整个隐藏；全部视图不变', async () => {
    withQuietFeeds();
    const reader = await started();
    expect(ids(reader)).toEqual({
      categories: [['user/-/label/科技', ['feed/1']]],
      feeds: ['feed/2'],
    });

    await reader.setView('all');
    expect(reader.sidebarTree).toEqual(be.tree);
  });

  it('选中的源读到零未读仍保留，选了别的项才隐藏', async () => {
    const reader = await started();
    await reader.setStream('feed/2');
    await reader.open(20);
    expect(reader.unreadIn('feed/2')).toBe(0);
    expect(ids(reader).feeds).toEqual(['feed/2']);

    await reader.setStream(READING_LIST);
    expect(ids(reader).feeds).toEqual([]);
  });

  it('选中的分类读到零未读仍保留，切换视图后才隐藏', async () => {
    const reader = await started();
    await reader.setStream('user/-/label/科技');
    await reader.markStream('user/-/label/科技');
    expect(reader.unreadIn('user/-/label/科技')).toBe(0);
    expect(ids(reader).categories).toEqual([['user/-/label/科技', []]]);

    await reader.setView('all');
    await reader.setView('unread');
    expect(ids(reader).categories).toEqual([]);
  });
});
