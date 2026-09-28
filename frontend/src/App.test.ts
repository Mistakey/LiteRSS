import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App.vue';
import { FakeBackend, feed, settle } from './test/fakeBackend';
import { useReader } from './stores/reader';

let be: FakeBackend;

beforeEach(() => {
  const pinia = createPinia();
  setActivePinia(pinia);
  be = new FakeBackend();
  be.tree = {
    categories: [{ id: 'user/-/label/科技', label: '科技', feeds: [feed('feed/1', '少数派')] }],
    feeds: [feed('feed/2', 'The Verge')],
  };
  be.add(
    { id: 30, stream: 'feed/1', title: '中文标题', translated: '中文标题', excerpt: '中文摘录' },
    {
      id: 20,
      stream: 'feed/2',
      title: 'English title',
      translated: '英文译文',
      excerpt: 'english',
      image: 'https://img/x.jpg',
    },
    { id: 10, stream: 'feed/1', title: '第三篇', translated: '第三篇' }
  );
  be.install();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function mounted() {
  const wrapper = mount(App, { attachTo: document.body });
  await settle();
  return wrapper;
}

describe('侧栏', () => {
  it('全部订阅 → 分类 → 源，带实时未读数', async () => {
    const w = await mounted();
    const nodes = w
      .findAll('.tree .node')
      .map((n) => `${n.find('.name').text()} ${n.find('.cnt').text()}`);
    expect(nodes).toEqual(['全部订阅 3', '科技 2', '少数派 2', 'The Verge 1']);
    expect(w.find('.seg button').text()).toBe('未读 3');
    w.unmount();
  });

  it('折叠分类隐藏其下的源', async () => {
    const w = await mounted();
    await w.find('.chev-btn').trigger('click');
    expect(w.findAll('.tree .node')).toHaveLength(3);
    w.unmount();
  });

  it('右键源「全部标为已读」后出现可撤销的 snackbar', async () => {
    const w = await mounted();
    await w.findAll('.tree .node')[3].trigger('contextmenu', { clientX: 10, clientY: 10 });
    const item = w.find('.ctx button');
    expect(item.text()).toContain('全部标为已读');
    expect(item.text()).toContain('1 篇');
    await item.trigger('click');
    await settle();
    expect(w.find('.snackbar').text()).toContain('「The Verge」1 篇已标为已读');
    expect(w.find('.snackbar button').text()).toBe('撤销');
    w.unmount();
  });

  it('未读视图里没选中的源读到零未读后从侧栏消失', async () => {
    const w = await mounted();
    await w.findAll('.tree .node')[3].trigger('contextmenu', { clientX: 10, clientY: 10 });
    await w.find('.ctx button').trigger('click');
    await settle();
    const names = w.findAll('.tree .node .name').map((n) => n.text());
    expect(names).toEqual(['全部订阅', '科技', '少数派']);
    w.unmount();
  });
});

describe('列表', () => {
  it('英文只显示译文，中文才显示摘录，有图才显示缩略图', async () => {
    const w = await mounted();
    const rows = w.findAll('.row');
    expect(rows.map((r) => r.find('.t').text())).toEqual(['中文标题', '英文译文', '第三篇']);
    expect(rows[0].find('.sn').text()).toBe('中文摘录');
    expect(rows[1].find('.sn').exists()).toBe(false);
    expect(rows[1].find('.chip').text()).toBe('译');
    expect(rows[1].find('img.thumb').exists()).toBe(true);
    expect(rows[0].find('img.thumb').exists()).toBe(false);
    w.unmount();
  });

  it('点开变灰但不消失', async () => {
    const w = await mounted();
    await w.findAll('.row')[1].trigger('click');
    await settle();
    const rows = w.findAll('.row');
    expect(rows).toHaveLength(3);
    expect(rows[1].classes()).toEqual(expect.arrayContaining(['read', 'on']));
    expect(w.find('.seg button').text()).toBe('未读 2');
    w.unmount();
  });

  it('右键「此篇及以上标为已读」', async () => {
    const w = await mounted();
    await w.findAll('.row')[1].trigger('contextmenu', { clientX: 5, clientY: 5 });
    const items = w.findAll('.ctx button');
    expect(items.map((b) => b.text().replace(/\s+/g, ' '))).toEqual([
      '标为已读',
      '此篇及以上标为已读 2 篇',
      '此篇及以下标为已读 2 篇',
      '在浏览器打开',
    ]);
    await items[1].trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/articles/read')[0].body).toEqual({ ids: [30, 20] });
    expect(w.findAll('.row.read')).toHaveLength(2);
    w.unmount();
  });

  it('新文章横幅点击才载入', async () => {
    const w = await mounted();
    be.add({ id: 40, stream: 'feed/2', title: '新到', translated: '新到' });
    await useReader().afterSync();
    await settle();
    expect(w.findAll('.row')).toHaveLength(3);
    const banner = w.find('.new-banner');
    expect(banner.text()).toBe('↑ 1 篇新文章，点击载入');
    await banner.trigger('click');
    await settle();
    expect(w.findAll('.row')).toHaveLength(4);
    expect(w.find('.new-banner').exists()).toBe(false);
    w.unmount();
  });

  it('切到「全部」视图', async () => {
    be.byId(10)!.read = true;
    const w = await mounted();
    expect(w.find('.scope-line').text()).toContain('未读 2 篇');
    await w.findAll('.seg button')[1].trigger('click');
    await settle();
    expect(w.find('.scope-line').text()).toContain('全部 3 篇');
    w.unmount();
  });
});

describe('设置入口', () => {
  it('侧栏没有底栏；顶栏应用菜单「设置…」打开设置面板，关闭后消失', async () => {
    const w = await mounted();
    expect(w.find('.sidebar .sb-foot').exists()).toBe(false);
    expect(w.find('.sidebar').text()).not.toContain('同步');
    expect(w.find('[role=dialog]').exists()).toBe(false);
    await w.find('.titlebar .app-btn').trigger('click');
    const item = w.findAll('.ctx [role=menuitem]').find((b) => b.text() === '设置…');
    await item!.trigger('click');
    await settle();
    expect(w.find('[role=dialog] h2').text()).toBe('设置');
    await w.find('[role=dialog] .head .icon-btn').trigger('click');
    expect(w.find('[role=dialog]').exists()).toBe(false);
    w.unmount();
  });
});
