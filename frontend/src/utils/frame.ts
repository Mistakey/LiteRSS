/**
 * 无边框窗口的宿主通道（spec D4）。拖动与边缘缩放直接给宿主发 Wails 认得的消息，不引入 Wails 运行时包；
 * 浏览器取证通道里没有宿主，这里的一切都不生效。
 */

type Post = (message: string) => void;

export interface Host {
  /** windows：WebView2，窗口按钮与边缘缩放由前端画；mac：WKWebView，系统红绿灯在顶栏左上。 */
  platform: 'windows' | 'mac';
  post: Post;
}

interface HostGlobals {
  chrome?: { webview?: { postMessage?: (m: unknown) => void } };
  webkit?: { messageHandlers?: { external?: { postMessage?: (m: unknown) => void } } };
}

export function detectHost(w: Window = window): Host | null {
  const g = w as unknown as HostGlobals;
  const webview = g.chrome?.webview;
  if (webview?.postMessage) return { platform: 'windows', post: (m) => webview.postMessage!(m) };
  const external = g.webkit?.messageHandlers?.external;
  if (external?.postMessage) return { platform: 'mac', post: (m) => external.postMessage!(m) };
  return null;
}

/** 无边框窗口铺满工作区即为最大化；系统阴影在窗口外，不占页面尺寸。 */
export function isMaximised(w: Window = window): boolean {
  return w.outerWidth >= w.screen.availWidth && w.outerHeight >= w.screen.availHeight;
}

export type Edge =
  | 'n-resize'
  | 'ne-resize'
  | 'e-resize'
  | 'se-resize'
  | 's-resize'
  | 'sw-resize'
  | 'w-resize'
  | 'nw-resize';

const BORDER = 5;
const CORNER = 15;

const cursors: Record<Edge, string> = {
  'n-resize': 'ns-resize',
  's-resize': 'ns-resize',
  'e-resize': 'ew-resize',
  'w-resize': 'ew-resize',
  'ne-resize': 'nesw-resize',
  'sw-resize': 'nesw-resize',
  'nw-resize': 'nwse-resize',
  'se-resize': 'nwse-resize',
};

/**
 * 指针在窗口边缘的哪一段。边宽 BORDER；贴着一条边、离相邻边不到 CORNER 时算角，
 * 所以角是 L 形，不会吃掉右上角关闭按钮的里侧。
 */
export function edgeAt(x: number, y: number, width: number, height: number): Edge | null {
  const [left, right, top, bottom] = [x, width - x - 1, y, height - y - 1];
  const near = (d: number) => d < BORDER;
  const band = (d: number) => d < CORNER;
  if ((near(bottom) && band(right)) || (near(right) && band(bottom))) return 'se-resize';
  if ((near(bottom) && band(left)) || (near(left) && band(bottom))) return 'sw-resize';
  if ((near(top) && band(left)) || (near(left) && band(top))) return 'nw-resize';
  if ((near(top) && band(right)) || (near(right) && band(top))) return 'ne-resize';
  if (near(left)) return 'w-resize';
  if (near(top)) return 'n-resize';
  if (near(bottom)) return 's-resize';
  if (near(right)) return 'e-resize';
  return null;
}

/**
 * Windows 无边框窗口的边缘缩放：指针进边缘时换光标，在边缘按下后移动发 `wails:resize:<边>`，
 * 由窗口进入系统的缩放循环。最大化时不缩放。返回卸载函数。
 */
export function installEdgeResize(host: Host, w: Window = window): () => void {
  let edge: Edge | null = null;
  let armed = false;
  const body = w.document.body;

  const onMove = (e: MouseEvent) => {
    if (armed && edge && e.buttons & 1) {
      armed = false;
      host.post(`wails:resize:${edge}`);
      return;
    }
    const next = isMaximised(w) ? null : edgeAt(e.clientX, e.clientY, w.innerWidth, w.innerHeight);
    if (next !== edge) body.style.cursor = next ? cursors[next] : '';
    edge = next;
  };
  const onDown = (e: MouseEvent) => {
    if (e.button !== 0 || !edge) return;
    // 边缘的按下属于窗口框，不交给下面的元素（比如列表行或顶栏拖动）。
    armed = true;
    e.preventDefault();
    e.stopPropagation();
  };
  const onUp = () => {
    armed = false;
  };

  w.addEventListener('mousemove', onMove, true);
  w.addEventListener('mousedown', onDown, true);
  w.addEventListener('mouseup', onUp, true);
  return () => {
    w.removeEventListener('mousemove', onMove, true);
    w.removeEventListener('mousedown', onDown, true);
    w.removeEventListener('mouseup', onUp, true);
    body.style.cursor = '';
  };
}
