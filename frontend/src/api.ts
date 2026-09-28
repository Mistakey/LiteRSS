import type { SettingsData } from './types/settings.generated';

/**
 * 后端 `/api` 的类型化客户端。只用相对路径，所以同一份构建在 Wails 窗口和浏览器取证通道里都成立（spec D4）。
 * 路由与载荷的权威说明在 docs/ARCHITECTURE.md「API 路由」。
 */

export const READING_LIST = 'user/-/state/com.google/reading-list';

/** 标签（分类）的流 ID；它的范围含其下所有源。 */
export function isLabel(stream: string): boolean {
  return stream.startsWith('user/-/label/');
}

export type View = 'unread' | 'all';

export interface Snapshot {
  ids: number[];
  /** 成员里最大的条目 ID，即该视图「全部标为已读」的 ts；空视图为 0。 */
  newest: number;
}

export interface Card {
  id: number;
  stream_id: string;
  feed_title: string;
  url: string;
  title: string;
  /** 空为未判定；等于 title 表示已判定为中文（pitfall 12）。 */
  translated_title: string;
  image_url: string;
  published_at: number;
  read: boolean;
  excerpt: string;
}

export interface Feed {
  id: string;
  title: string;
  url: string;
  site_url: string;
  icon_url: string;
}

export interface Category {
  id: string;
  label: string;
  feeds: Feed[];
}

export interface Tree {
  categories: Category[];
  feeds: Feed[];
}

export interface Counts {
  total: number;
  feeds: Record<string, number>;
  tags: Record<string, number>;
}

export interface SyncState {
  rev: number;
  running: boolean;
  new_items: number;
  pending: number;
  /** 上次成功同步的 Unix 秒，0 为从未。 */
  last_sync_at: number;
  /** 上次失败的英文原因，成功后为空；界面文案由前端给。 */
  error: string;
  legacy_running: boolean;
}

export interface Batch {
  token: string;
  count: number;
}

export interface TitleTranslations {
  titles: { id: number; translated_title: string }[];
  message: string;
}

export interface FullText {
  /** success 或失败种类（blocked、no_content、no_link 等）。 */
  outcome: string;
  /** 抓到的全文，不可信 HTML，经 sanitizeArticleHtml 才进 DOM。 */
  content: string;
  /** 失败时可直接显示的中文原因。 */
  message: string;
}

export interface Summary {
  /** 渲染好的摘要，不可信 HTML；空表示没有生成。 */
  html: string;
  /** 中文说明：没有生成的原因，或摘要依据的不是全文。 */
  note: string;
}

export interface SettingsView {
  /** 面板编辑的全部键；凭据一律为空串，已存的列在 saved_secrets。 */
  settings: SettingsData;
  saved_secrets: (keyof SettingsData)[];
}

/** 测试连接的结果；message 是可直接显示的中文。 */
export interface ConnectionTest {
  ok: boolean;
  message: string;
}

export interface UpdateCheck {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  release_url: string;
  /** 能在应用内更新；否则（便携版等）只能去 release_url 下载。 */
  in_app: boolean;
  /** 检查失败时的中文原因。 */
  message: string;
}

/** 应用内更新的一步（spec D20）；not_installed 是开发构建校验通过后的终态。 */
export type UpdateState =
  'idle' | 'downloading' | 'verifying' | 'installing' | 'not_installed' | 'failed';

export interface UpdateStatus {
  state: UpdateState;
  version: string;
  /** 安装包已下载的字节数与总字节数（未知为 0）。 */
  received: number;
  total: number;
  /** 失败或开发构建停下时的中文说明。 */
  message: string;
  /** 失败时手动下载的发布页。 */
  release_url: string;
}

/** 请求失败：status 为 HTTP 状态码，网络错误为 0。 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string
  ) {
    super(message);
  }
}

async function request(path: string, init?: RequestInit): Promise<Response> {
  let res: Response;
  try {
    res = await fetch(path, init);
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err;
    throw new ApiError(0, String(err));
  }
  if (!res.ok) throw new ApiError(res.status, (await res.text()).trim());
  return res;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  return (await request(path, { signal })).json() as Promise<T>;
}

async function post(path: string, body?: unknown): Promise<Response> {
  return request(path, {
    method: 'POST',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

async function postJSON<T>(path: string, body?: unknown): Promise<T> {
  return (await post(path, body)).json() as Promise<T>;
}

/** 每次取卡片的上限（后端 library.MaxCards）。 */
export const MAX_CARDS = 200;

export const api = {
  snapshot: (view: View, stream: string) =>
    getJSON<Snapshot>(`/api/articles?${new URLSearchParams({ view, stream }).toString()}`),
  cards: (ids: number[]) =>
    ids.length ? getJSON<Card[]>(`/api/articles/cards?ids=${ids.join(',')}`) : Promise.resolve([]),
  counts: () => getJSON<Counts>('/api/unread-counts'),
  tree: () => getJSON<Tree>('/api/subscriptions'),

  setRead: async (id: number, read: boolean) => {
    await post(`/api/articles/${id}/read`, { read });
  },
  markItemsRead: (ids: number[]) => postJSON<Batch>('/api/articles/read', { ids }),
  /** ts 为 0 时后端取该范围本地最新条目。 */
  markStreamRead: (stream: string, ts: number) =>
    postJSON<Batch>('/api/streams/read', ts ? { stream, ts } : { stream }),
  undo: async (token: string) => {
    await post('/api/undo', { token });
  },

  /** RSS 正文原样，不可信 HTML；没有正文为空串。 */
  content: async (id: number) =>
    (await getJSON<{ content: string }>(`/api/articles/${id}/content`)).content,
  fullText: (id: number) => postJSON<FullText>(`/api/articles/${id}/fulltext`),
  summary: (id: number) => postJSON<Summary>(`/api/articles/${id}/summary`),

  translateTitles: (ids: number[]) =>
    postJSON<TitleTranslations>('/api/articles/translate-titles', { ids }),
  openInBrowser: async (url: string) => {
    await post('/api/browser/open', { url });
  },

  settings: () => getJSON<SettingsView>('/api/settings'),
  /** 只写给出的键；回写整份对象会用空串清掉已存凭据。 */
  updateSettings: (changes: Partial<SettingsData>) =>
    postJSON<SettingsView>('/api/settings/update', changes),
  /** 省略的字段后端取已存值，所以没改的凭据不传。 */
  testFreshRSS: (form: Partial<SettingsData>) =>
    postJSON<ConnectionTest>('/api/settings/freshrss/test', form),
  testModel: (form: Partial<SettingsData>) =>
    postJSON<ConnectionTest>('/api/settings/llm/test', form),
  version: async () => (await getJSON<{ version: string }>('/api/version')).version,
  checkUpdate: () => getJSON<UpdateCheck>('/api/update/check'),
  /** 开始应用内更新；已在进行时后端原样返回当前进度。 */
  startUpdate: () => postJSON<UpdateStatus>('/api/update/start'),
  updateStatus: () => getJSON<UpdateStatus>('/api/update/status'),

  syncState: (since: number, signal?: AbortSignal) =>
    getJSON<SyncState>(`/api/sync/state?since=${since}`, signal),
  syncNow: async () => {
    await post('/api/sync/run');
  },

  /** 无边框窗口的三个按钮；关闭与点 × 相同，按 close_to_tray 藏到托盘或退出。 */
  window: {
    minimise: async () => {
      await post('/api/window/minimise');
    },
    /** 最大化，已最大化时还原；Windows 上双击顶栏也调它。 */
    toggleMaximise: async () => {
      await post('/api/window/maximise');
    },
    close: async () => {
      await post('/api/window/close');
    },
  },
};
