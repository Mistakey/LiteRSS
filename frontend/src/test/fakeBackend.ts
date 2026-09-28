/**
 * 测试用的内存后端：按 docs/ARCHITECTURE.md「API 路由」的形状回应前端用到的 `/api` 路由，
 * 装进 globalThis.fetch。只模拟前端看得见的规则（快照顺序、显示状态、撤销令牌），不模拟同步。
 */
import { vi } from 'vitest';
import {
  READING_LIST,
  type Card,
  type ConnectionTest,
  type FullText,
  type Summary,
  type SyncState,
  type Tree,
  type UpdateCheck,
  type UpdateStatus,
} from '../api';
import { settingsDefaults, type SettingsData } from '../types/settings.generated';

const SECRETS: (keyof SettingsData)[] = [
  'freshrss_api_password',
  'llm_api_key',
  'baidu_secret_key',
  'proxy_username',
  'proxy_password',
];

export interface FakeArticle {
  id: number;
  stream: string;
  title: string;
  translated?: string;
  url?: string;
  published?: number;
  read?: boolean;
  excerpt?: string;
  image?: string;
  /** RSS 正文，缺省为空串。 */
  content?: string;
}

interface Call {
  method: string;
  path: string;
  body: unknown;
}

export class FakeBackend {
  articles: FakeArticle[] = [];
  tree: Tree = { categories: [], feeds: [] };
  calls: Call[] = [];
  /** 下一次 translate-titles 回应的译文（按 ID），没有的留在未判定。 */
  translations: Record<number, string> = {};
  syncStates: SyncState[] = [];
  /** 抓全文的结果（按 ID）；没有的回 no_link。 */
  fullTexts: Record<number, FullText> = {};
  /** 已缓存的全文（按 ID），随正文一起返回；抓全文成功时写入。 */
  cachedFullTexts: Record<number, string> = {};
  /** 摘要的结果（按 ID）；没有的回「还没有配置摘要模型」。 */
  summaries: Record<number, Summary> = {};
  /** 已存的设置，凭据按明文存；GET 时凭据回空串、列进 saved_secrets。 */
  settings: SettingsData = { ...settingsDefaults };
  /** 两个测试连接的回应；缺省为成功。 */
  tests: Partial<Record<'freshrss' | 'llm', ConnectionTest>> = {};
  update: UpdateCheck = {
    current_version: '0.1.0',
    latest_version: '0.1.0',
    update_available: false,
    release_url: 'https://github.com/example/literss/releases',
    in_app: false,
    message: '',
  };
  /** 应用内更新依次报告的进度：start 回第一项，之后每次 status 前进一项并停在最后一项。 */
  updateSteps: UpdateStatus[] = [];
  private updateStep = -1;
  private batches = new Map<string, number[]>();
  private seq = 0;

  add(...articles: FakeArticle[]) {
    this.articles.push(...articles);
  }

  byId(id: number) {
    return this.articles.find((a) => a.id === id);
  }

  inStream(a: FakeArticle, stream: string) {
    if (stream === READING_LIST || a.stream === stream) return true;
    const cat = this.tree.categories.find((c) => c.id === stream);
    return !!cat?.feeds.some((f) => f.id === a.stream);
  }

  callsTo(method: string, path: string) {
    return this.calls.filter((c) => c.method === method && c.path === path);
  }

  install() {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: string, init?: RequestInit) => this.handle(input, init))
    );
  }

  private async handle(input: string, init?: RequestInit): Promise<Response> {
    const url = new URL(input, 'http://127.0.0.1:1235');
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    this.calls.push({ method, path: url.pathname, body });
    const q = url.searchParams;
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { 'Content-Type': 'application/json' } });
    const none = () => new Response(null, { status: 204 });
    const route = `${method} ${url.pathname}`;

    if (route === 'GET /api/articles') {
      const stream = q.get('stream') ?? READING_LIST;
      const ids = this.articles
        .filter((a) => this.inStream(a, stream) && (q.get('view') === 'all' || !a.read))
        .sort((a, b) => (b.published ?? b.id) - (a.published ?? a.id) || b.id - a.id)
        .map((a) => a.id);
      return json({ ids, newest: ids.length ? Math.max(...ids) : 0 });
    }
    if (route === 'GET /api/articles/cards') {
      const ids = (q.get('ids') ?? '').split(',').map(Number);
      return json(ids.flatMap((id) => (this.byId(id) ? [this.card(this.byId(id)!)] : [])));
    }
    if (route === 'GET /api/unread-counts') {
      const unread = this.articles.filter((a) => !a.read);
      const feeds: Record<string, number> = {};
      for (const a of unread) feeds[a.stream] = (feeds[a.stream] ?? 0) + 1;
      const tags: Record<string, number> = {};
      for (const c of this.tree.categories) {
        const n = unread.filter((a) => this.inStream(a, c.id)).length;
        if (n) tags[c.id] = n;
      }
      return json({ total: unread.length, feeds, tags });
    }
    if (route === 'GET /api/subscriptions') return json(this.tree);
    if (route === 'GET /api/sync/state') {
      const next = this.syncStates.shift();
      if (!next) return new Promise(() => {}); // 挂起，像没有变化的长轮询
      return json(next);
    }
    const one = url.pathname.match(/^\/api\/articles\/(\d+)\/read$/);
    if (method === 'POST' && one) {
      const a = this.byId(Number(one[1]));
      if (a) a.read = (body as { read: boolean }).read;
      return none();
    }
    if (route === 'POST /api/articles/read') {
      return json(this.batch((body as { ids: number[] }).ids));
    }
    if (route === 'POST /api/streams/read') {
      const { stream, ts } = body as { stream: string; ts?: number };
      const ids = this.articles
        .filter((a) => this.inStream(a, stream) && (!ts || a.id <= ts))
        .map((a) => a.id);
      return json(this.batch(ids));
    }
    if (route === 'POST /api/undo') {
      const ids = this.batches.get((body as { token: string }).token);
      if (!ids) return new Response('undo expired', { status: 410 });
      this.batches.delete((body as { token: string }).token);
      for (const id of ids) this.byId(id)!.read = false;
      return none();
    }
    const content = url.pathname.match(/^\/api\/articles\/(\d+)\/content$/);
    if (method === 'GET' && content) {
      const a = this.byId(Number(content[1]));
      return a
        ? json({ content: a.content ?? '', fulltext: this.cachedFullTexts[a.id] ?? '' })
        : new Response('not found', { status: 404 });
    }
    const action = url.pathname.match(/^\/api\/articles\/(\d+)\/(fulltext|summary)$/);
    if (method === 'POST' && action) {
      const id = Number(action[1]);
      if (action[2] === 'fulltext') {
        const ft = this.fullTexts[id] ?? {
          outcome: 'no_link',
          content: '',
          message: '这篇文章没有可抓取的原文链接。',
        };
        if (ft.outcome === 'success') this.cachedFullTexts[id] = ft.content;
        return json(ft);
      }
      return json(this.summaries[id] ?? { html: '', note: '还没有配置摘要模型，请在设置里填写。' });
    }
    if (route === 'POST /api/articles/translate-titles') {
      const titles = (body as { ids: number[] }).ids.flatMap((id) => {
        const t = this.translations[id];
        if (t === undefined) return [];
        this.byId(id)!.translated = t;
        return [{ id, translated_title: t }];
      });
      return json({ titles, message: '' });
    }
    if (route === 'GET /api/settings') return json(this.settingsView());
    if (route === 'POST /api/settings/update') {
      const changes = body as Record<string, unknown>;
      const unknown = Object.keys(changes).filter((k) => !(k in settingsDefaults));
      if (unknown.length) return new Response(`unknown settings: ${unknown}`, { status: 400 });
      const emptied = SECRETS.filter((k) => changes[k] === '');
      if (emptied.length) return new Response(`clear ${emptied} explicitly`, { status: 400 });
      Object.assign(this.settings, changes);
      return json(this.settingsView());
    }
    if (route === 'POST /api/settings/secrets/clear') {
      const { key } = body as { key: keyof SettingsData };
      if (!SECRETS.includes(key))
        return new Response(`${key} is not a credential`, { status: 400 });
      (this.settings as Record<string, unknown>)[key] = '';
      return json(this.settingsView());
    }
    const test = url.pathname.match(/^\/api\/settings\/(freshrss|llm)\/test$/);
    if (method === 'POST' && test) {
      return json(this.tests[test[1] as 'freshrss' | 'llm'] ?? { ok: true, message: '连接成功。' });
    }
    if (route === 'GET /api/version') return json({ version: this.update.current_version });
    if (route === 'GET /api/update/check') return json(this.update);
    if (route === 'POST /api/update/start') {
      this.updateStep = 0;
      return json(this.updateStatus(), 202);
    }
    if (route === 'GET /api/update/status') {
      if (this.updateStep >= 0)
        this.updateStep = Math.min(this.updateStep + 1, this.updateSteps.length - 1);
      return json(this.updateStatus());
    }
    if (route === 'POST /api/sync/run') return new Response(null, { status: 202 });
    if (route === 'POST /api/browser/open') return none();
    if (method === 'POST' && /^\/api\/window\/(minimise|maximise|close)$/.test(url.pathname)) {
      return none();
    }
    return new Response('not found', { status: 404 });
  }

  private updateStatus(): UpdateStatus {
    const idle: UpdateStatus = {
      state: 'idle',
      version: '',
      received: 0,
      total: 0,
      message: '',
      release_url: '',
    };
    return this.updateSteps[this.updateStep] ?? idle;
  }

  private settingsView() {
    const settings = { ...this.settings } as Record<string, unknown>;
    for (const k of SECRETS) settings[k] = '';
    return { settings, saved_secrets: SECRETS.filter((k) => this.settings[k] !== '') };
  }

  private batch(ids: number[]) {
    const changed = ids.filter((id) => {
      const a = this.byId(id);
      if (!a || a.read) return false;
      a.read = true;
      return true;
    });
    const token = `t${++this.seq}`;
    this.batches.set(token, changed);
    return { token, count: changed.length };
  }

  private card(a: FakeArticle): Card {
    return {
      id: a.id,
      stream_id: a.stream,
      feed_title: this.feedTitle(a.stream),
      url: a.url ?? `https://example.com/${a.id}`,
      title: a.title,
      translated_title: a.translated ?? '',
      image_url: a.image ?? '',
      published_at: a.published ?? a.id,
      read: !!a.read,
      excerpt: a.excerpt ?? '',
    };
  }

  private feedTitle(stream: string) {
    const all = [...this.tree.feeds, ...this.tree.categories.flatMap((c) => c.feeds)];
    return all.find((f) => f.id === stream)?.title ?? '';
  }
}

export function feed(id: string, title: string) {
  return { id, title, url: '', site_url: '', icon_url: '' };
}

/** 让挂在微任务与已完成 fetch 上的后续步骤跑完。 */
export async function settle() {
  for (let i = 0; i < 10; i++) await new Promise((r) => setTimeout(r, 0));
}
