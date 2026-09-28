import { mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import TitleBar from './TitleBar.vue';
import { FakeBackend, settle } from '../test/fakeBackend';

let be: FakeBackend;
let post: ReturnType<typeof vi.fn<(m: string) => void>>;

beforeEach(() => {
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
  it('浏览器取证通道：只有图标与名称，没有窗口按钮', () => {
    const w = mounted();
    expect(w.find('.logo').exists()).toBe(true);
    expect(w.find('.logo').attributes('srcset')).toContain('/assets/logo-24.svg 1.5x');
    expect(w.find('.name').text()).toBe('LiteRSS');
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
    const name = w.find('.name');
    await name.trigger('mousedown', { button: 0, buttons: 1 });
    expect(post).not.toHaveBeenCalled();
    await name.trigger('mousemove', { buttons: 1 });
    await name.trigger('mousemove', { buttons: 1 });
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
    const close = w.find('.win-buttons .close');
    await close.trigger('mousedown', { button: 0, buttons: 1 });
    await close.trigger('mousemove', { buttons: 1 });
    expect(post).not.toHaveBeenCalled();
    w.unmount();
  });

  it('Windows 双击顶栏走最大化接口，双击按钮不算', async () => {
    inWebView2();
    const w = mounted();
    await w.find('.name').trigger('dblclick');
    await w.find('.win-buttons button').trigger('dblclick');
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
