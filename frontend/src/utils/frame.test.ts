import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { detectHost, edgeAt, installEdgeResize } from './frame';

describe('detectHost', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('浏览器取证通道里没有宿主', () => {
    expect(detectHost()).toBeNull();
  });

  it('WebView2 是 windows，WKWebView 是 mac，消息原样转给宿主', () => {
    const webview = vi.fn();
    vi.stubGlobal('chrome', { webview: { postMessage: webview } });
    const host = detectHost()!;
    expect(host.platform).toBe('windows');
    host.post('wails:drag');
    expect(webview).toHaveBeenCalledWith('wails:drag');

    vi.unstubAllGlobals();
    const external = vi.fn();
    vi.stubGlobal('webkit', { messageHandlers: { external: { postMessage: external } } });
    expect(detectHost()!.platform).toBe('mac');
  });
});

describe('edgeAt', () => {
  const at = (x: number, y: number) => edgeAt(x, y, 1000, 800);

  it('四条边各 5px', () => {
    expect([at(0, 400), at(4, 400), at(5, 400)]).toEqual(['w-resize', 'w-resize', null]);
    expect([at(500, 0), at(500, 799), at(999, 400)]).toEqual(['n-resize', 's-resize', 'e-resize']);
    expect(at(500, 400)).toBeNull();
  });

  it('角是 L 形：贴边且离相邻边不到 15px', () => {
    expect([at(0, 0), at(14, 0), at(0, 14)]).toEqual(['nw-resize', 'nw-resize', 'nw-resize']);
    expect([at(999, 0), at(985, 2), at(997, 10)]).toEqual(['ne-resize', 'ne-resize', 'ne-resize']);
    expect([at(999, 799), at(0, 799)]).toEqual(['se-resize', 'sw-resize']);
  });

  it('右上角关闭按钮的里侧不算边缘', () => {
    expect(at(990, 10)).toBeNull();
  });
});

describe('installEdgeResize', () => {
  let post: ReturnType<typeof vi.fn<(m: string) => void>>;
  let remove: () => void;

  const screen = (width: number, height: number) => {
    Object.defineProperty(window.screen, 'availWidth', { value: width, configurable: true });
    Object.defineProperty(window.screen, 'availHeight', { value: height, configurable: true });
  };
  const mouse = (type: string, x: number, y: number, buttons = 0) => {
    const e = new MouseEvent(type, {
      clientX: x,
      clientY: y,
      buttons,
      bubbles: true,
      cancelable: true,
    });
    document.body.dispatchEvent(e);
    return e;
  };

  beforeEach(() => {
    screen(4000, 4000); // 窗口没铺满工作区
    post = vi.fn();
    remove = installEdgeResize({ platform: 'windows', post });
  });
  afterEach(() => {
    remove();
    screen(0, 0);
  });

  it('指针进边缘换光标，按下后移动才发 wails:resize', () => {
    mouse('mousemove', 1, 300);
    expect(document.body.style.cursor).toBe('ew-resize');
    const down = mouse('mousedown', 1, 300, 1);
    expect(down.defaultPrevented).toBe(true);
    expect(post).not.toHaveBeenCalled();
    mouse('mousemove', 2, 300, 1);
    expect(post.mock.calls).toEqual([['wails:resize:w-resize']]);
  });

  it('离开边缘恢复光标，内部按下不拦截', () => {
    mouse('mousemove', 1, 300);
    mouse('mousemove', 300, 300);
    expect(document.body.style.cursor).toBe('');
    expect(mouse('mousedown', 300, 300, 1).defaultPrevented).toBe(false);
    mouse('mousemove', 301, 300, 1);
    expect(post).not.toHaveBeenCalled();
  });

  it('最大化时不缩放', () => {
    screen(100, 100);
    mouse('mousemove', 1, 300);
    expect(document.body.style.cursor).toBe('');
    mouse('mousedown', 1, 300, 1);
    mouse('mousemove', 2, 300, 1);
    expect(post).not.toHaveBeenCalled();
  });
});
