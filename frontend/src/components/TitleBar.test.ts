import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import TitleBar from './TitleBar.vue';
import type { SyncState } from '../api';
import { useSync } from '../stores/sync';
import { FakeBackend, settle } from '../test/fakeBackend';

let be: FakeBackend;
let post: ReturnType<typeof vi.fn<(m: string) => void>>;

beforeEach(() => {
  setActivePinia(createPinia());
  be = new FakeBackend();
  be.install();
  post = vi.fn();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const inWebView2 = () => vi.stubGlobal('chrome', { webview: { postMessage: post } });
const inWKWebView = () =>
  vi.stubGlobal('webkit', { messageHandlers: { external: { postMessage: post } } });

function mounted() {
  return mount(TitleBar, { attachTo: document.body });
}

describe('顶栏', () => {
  it('浏览器取证通道：应用菜单按钮与同步状态，没有窗口按钮', () => {
    const w = mounted();
    const app = w.find('.app-btn');
    expect(app.find('.logo').exists()).toBe(true);
    expect(app.find('.logo').attributes('srcset')).toContain('/assets/logo-24.svg 1.5x');
    expect(app.find('.name').text()).toBe('LiteRSS');
    expect(w.find('.sync-state').text()).toBe('正在连接…');
    expect(w.find('.win-buttons').exists()).toBe(false);
    w.unmount();
  });

  it('WebView2 里有最小化、最大化、关闭，各调 /api/window', async () => {
    inWebView2();
    const w = mounted();
    const buttons = w.findAll('.win-buttons button');
    expect(buttons.map((b) => b.attributes('aria-label'))).toEqual(['最小化', '最大化', '关闭']);
    for (const b of buttons) await b.trigger('click');
    await settle();
    expect(be.calls.filter((c) => c.method === 'POST').map((c) => c.path)).toEqual([
      '/api/window/minimise',
      '/api/window/maximise',
      '/api/window/close',
    ]);
    expect(post).not.toHaveBeenCalled();
    w.unmount();
  });

  it('拖动区按下后移动发 wails:drag，一次按下只发一次', async () => {
    inWebView2();
    const w = mounted();
    const status = w.find('.sync-state');
    await status.trigger('mousedown', { button: 0, buttons: 1 });
    expect(post).not.toHaveBeenCalled();
    await status.trigger('mousemove', { buttons: 1 });
    await status.trigger('mousemove', { buttons: 1 });
    expect(post.mock.calls).toEqual([['wails:drag']]);
    w.unmount();
  });

  it('按下不移动、在按钮上按下、没按住移动，都不拖', async () => {
    inWebView2();
    const w = mounted();
    const bar = w.find('.titlebar');
    await bar.trigger('mousedown', { button: 0, buttons: 1 });
    await bar.trigger('mouseup', { button: 0 });
    await bar.trigger('mousemove', { buttons: 0 });
    for (const sel of ['.win-buttons .close', '.app-btn .name']) {
      const btn = w.find(sel);
      await btn.trigger('mousedown', { button: 0, buttons: 1 });
      await btn.trigger('mousemove', { buttons: 1 });
    }
    expect(post).not.toHaveBeenCalled();
    w.unmount();
  });

  it('Windows 双击顶栏走最大化接口，双击按钮不算', async () => {
    inWebView2();
    const w = mounted();
    await w.find('.sync-state').trigger('dblclick');
    await w.find('.win-buttons button').trigger('dblclick');
    await w.find('.app-btn').trigger('dblclick');
    await settle();
    expect(be.callsTo('POST', '/api/window/maximise')).toHaveLength(1);
    w.unmount();
  });

  it('macOS：没有自画的窗口按钮，拖动与双击都交给宿主', async () => {
    inWKWebView();
    const w = mounted();
    expect(w.find('.win-buttons').exists()).toBe(false);
    const bar = w.find('.titlebar');
    expect(bar.classes()).toContain('mac');
    await bar.trigger('mousedown', { button: 0, buttons: 1 });
    await bar.trigger('mousemove', { buttons: 1 });
    await bar.trigger('dblclick');
    await settle();
    expect(post.mock.calls).toEqual([['wails:drag'], ['wails:drag:doubleclick']]);
    expect(be.calls.filter((c) => c.path.startsWith('/api/window'))).toHaveLength(0);
    w.unmount();
  });

  it('浏览器取证通道里按下移动与双击什么都不做', async () => {
    const w = mounted();
    const bar = w.find('.titlebar');
    await bar.trigger('mousedown', { button: 0, buttons: 1 });
    await bar.trigger('mousemove', { buttons: 1 });
    await bar.trigger('dblclick');
    await settle();
    expect(be.calls).toHaveLength(0);
    w.unmount();
  });
});

const synced: SyncState = {
  rev: 1,
  running: false,
  new_items: 0,
  pending: 0,
  last_sync_at: Math.floor(Date.now() / 1000),
  error: '',
};

function menuItems() {
  return [...document.querySelectorAll<HTMLButtonElement>('.ctx [role=menuitem]')];
}

describe('应用菜单', () => {
  it('点图标与名称展开：立即同步（附同步状态）与设置…，再点收起', async () => {
    useSync().state = synced;
    const w = mounted();
    expect(w.find('.sync-state').text()).toBe('刚刚已同步');
    const app = w.find('.app-btn');
    expect(app.attributes('aria-expanded')).toBe('false');
    await app.trigger('click');
    expect(app.attributes('aria-expanded')).toBe('true');
    const items = menuItems();
    expect(items.map((b) => b.textContent!.replace(/\s+/g, ' ').trim())).toEqual([
      '立即同步 刚刚已同步',
      '设置…',
    ]);
    expect(items[0].disabled).toBe(false);
    document
      .querySelector<HTMLElement>('.ctx')!
      .parentElement!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
    await settle();
    expect(menuItems()).toHaveLength(0);
    w.unmount();
  });

  it('立即同步调 /api/sync/run 并收起菜单', async () => {
    useSync().state = synced;
    const w = mounted();
    await w.find('.app-btn').trigger('click');
    menuItems()[0].click();
    await settle();
    expect(be.callsTo('POST', '/api/sync/run')).toHaveLength(1);
    expect(menuItems()).toHaveLength(0);
    w.unmount();
  });

  it('同步中：顶栏显示正在同步，菜单里立即同步置灰，点了不发请求', async () => {
    useSync().state = { ...synced, running: true };
    const w = mounted();
    expect(w.find('.sync-state').text()).toBe('正在同步…');
    await w.find('.app-btn').trigger('click');
    const [run] = menuItems();
    expect(run.disabled).toBe(true);
    expect(run.textContent).toContain('正在同步…');
    run.click();
    await settle();
    expect(be.callsTo('POST', '/api/sync/run')).toHaveLength(0);
    w.unmount();
  });

  it('设置…发出 settings', async () => {
    const w = mounted();
    await w.find('.app-btn').trigger('click');
    menuItems()[1].click();
    await settle();
    expect(w.emitted('settings')).toHaveLength(1);
    expect(menuItems()).toHaveLength(0);
    w.unmount();
  });
});
