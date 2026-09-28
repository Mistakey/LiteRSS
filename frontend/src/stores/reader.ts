/**
 * 阅读状态：视图与范围、列表快照与卡片、选中的文章、侧栏的订阅树与实时未读数（spec D8）。
 *
 * 快照是进入视图时的有序成员，此后固定：已读只变灰，同步来的新条目只进横幅（newCount），
 * 用户点横幅、切换视图或范围时才重新取。未读数是实时值，操作后与同步后都重新取。
 */
import { defineStore } from 'pinia';
import { computed, ref, shallowReactive } from 'vue';
import {
  api,
  MAX_CARDS,
  READING_LIST,
  type Card,
  type Counts,
  type Snapshot,
  type Tree,
  type View,
} from '../api';
import { useSnackbar } from './snackbar';

/** 每次向下滚动时载入的卡片数。 */
const PAGE = 100;

export const useReader = defineStore('reader', () => {
  const view = ref<View>('unread');
  const stream = ref(READING_LIST);
  const snapshot = ref<Snapshot>({ ids: [], newest: 0 });
  /** 快照前 loaded 个 ID 已请求过卡片。 */
  const loaded = ref(0);
  const cards = shallowReactive(new Map<number, Card>());
  const selectedId = ref<number | null>(null);
  /** 最近一次同步后，当前视图里不在快照中的条目。 */
  const fresh = ref<number[]>([]);
  const tree = ref<Tree>({ categories: [], feeds: [] });
  const counts = ref<Counts>({ total: 0, feeds: {}, tags: {} });
  const loading = ref(false);

  // 每次进入视图加一；慢到的旧响应据此丢弃。
  let epoch = 0;
  // 已请求过译文的条目：百度失败时同一批不反复重试（ARCHITECTURE「API 路由」）。
  const translateTried = new Set<number>();
  // 本地改显示状态的序号：卡片被改时记下当时的序号，改动之前发出的刷新不覆盖它。
  let mutation = 0;
  const touched = new Map<number, number>();

  const rows = computed(() =>
    snapshot.value.ids.slice(0, loaded.value).flatMap((id) => {
      const c = cards.get(id);
      return c ? [c] : [];
    })
  );
  const hasMore = computed(() => loaded.value < snapshot.value.ids.length);
  const newCount = computed(() => fresh.value.length);

  function streamName(s: string): string {
    if (s === READING_LIST) return '全部订阅';
    const cat = tree.value.categories.find((c) => c.id === s);
    if (cat) return cat.label;
    const all = [...tree.value.feeds, ...tree.value.categories.flatMap((c) => c.feeds)];
    return all.find((f) => f.id === s)?.title || '未知订阅源';
  }

  function unreadIn(s: string): number {
    if (s === READING_LIST) return counts.value.total;
    return counts.value.tags[s] ?? counts.value.feeds[s] ?? 0;
  }

  /** 组件触发的动作失败时给一句提示，不留下未处理的拒绝。 */
  async function guarded(what: string, run: () => Promise<void>) {
    try {
      await run();
    } catch {
      useSnackbar().show({ text: `${what}失败，请稍后重试` });
    }
  }

  async function init() {
    await guarded('载入', async () => {
      await Promise.all([refreshTree(), refreshCounts(), enter()]);
    });
  }

  async function refreshTree() {
    tree.value = await api.tree();
  }

  async function refreshCounts() {
    counts.value = await api.counts();
  }

  /** 取当前视图的快照并载入第一页卡片。 */
  async function enter() {
    const my = ++epoch;
    loading.value = true;
    try {
      const s = await api.snapshot(view.value, stream.value);
      if (my !== epoch) return;
      snapshot.value = s;
      loaded.value = 0;
      fresh.value = [];
      await loadPage();
    } finally {
      if (my === epoch) loading.value = false;
    }
  }

  async function setView(v: View) {
    view.value = v;
    selectedId.value = null;
    await guarded('载入列表', enter);
  }

  async function setStream(s: string) {
    stream.value = s;
    selectedId.value = null;
    await guarded('载入列表', enter);
  }

  // 每个视图同时只有一个翻页请求；失败时不前移 loaded，下次滚到底部重试。
  let pagingEpoch = -1;

  /** 滚到底部时载入下一页；视图还在换快照时不动，由 enter 载入第一页。 */
  async function loadMore() {
    if (!loading.value) await loadPage();
  }

  async function loadPage() {
    const my = epoch;
    if (pagingEpoch === my) return;
    const ids = snapshot.value.ids.slice(loaded.value, loaded.value + PAGE);
    if (!ids.length) return;
    pagingEpoch = my;
    try {
      const got = await fetchCards(ids);
      if (my !== epoch) return;
      loaded.value += ids.length;
      for (const c of got) cards.set(c.id, c);
      void translate(got);
    } catch {
      // 本地请求很少失败；列表停在已载入处，滚动时再试。
    } finally {
      if (pagingEpoch === my) pagingEpoch = -1;
    }
  }

  async function fetchCards(ids: number[]): Promise<Card[]> {
    const out: Card[] = [];
    for (let i = 0; i < ids.length; i += MAX_CARDS) {
      out.push(...(await api.cards(ids.slice(i, i + MAX_CARDS))));
    }
    return out;
  }

  /** 重新取已载入卡片的显示状态与译文，快照成员不变。 */
  async function refreshLoaded() {
    const my = epoch;
    const since = mutation;
    const got = await fetchCards(snapshot.value.ids.slice(0, loaded.value));
    if (my !== epoch) return;
    for (const c of got) {
      if ((touched.get(c.id) ?? 0) <= since) cards.set(c.id, c);
    }
  }

  // 未判定的标题（译文为空）才请求；已判定的译文等于原标题表示中文（pitfall 12）。
  async function translate(got: Card[]) {
    const ids = got
      .filter((c) => c.translated_title === '' && !translateTried.has(c.id))
      .map((c) => c.id);
    if (!ids.length) return;
    for (const id of ids) translateTried.add(id);
    try {
      const res = await api.translateTitles(ids);
      for (const t of res.titles) {
        const c = cards.get(t.id);
        if (c) cards.set(t.id, { ...c, translated_title: t.translated_title });
      }
    } catch {
      // 译文拿不到时列表显示原标题，不打扰用户。
    }
  }

  /** 已载入卡片里与 id 同 URL 的（含自身）一起改显示状态：后端按 URL 组写意图。 */
  function markLocal(ids: number[], read: boolean) {
    const seq = ++mutation;
    const want = new Set(ids);
    const urls = new Set(ids.flatMap((id) => cards.get(id)?.url || []));
    for (const [id, c] of cards) {
      if (want.has(id) || (c.url && urls.has(c.url))) {
        touched.set(id, seq);
        if (c.read !== read) cards.set(id, { ...c, read });
      }
    }
  }

  async function setRead(id: number, read: boolean) {
    markLocal([id], read);
    try {
      await api.setRead(id, read);
    } catch {
      markLocal([id], !read);
      useSnackbar().show({ text: read ? '标为已读失败，请稍后重试' : '标为未读失败，请稍后重试' });
      return;
    }
    // 意图写入之前发出的刷新可能带回旧状态，写入后再盖一次章。
    markLocal([id], read);
    await guarded('刷新未读数', refreshCounts);
  }

  /** 点开即已读（spec §1）。 */
  async function open(id: number) {
    selectedId.value = id;
    if (cards.get(id)?.read === false) await setRead(id, true);
  }

  function showBatch(label: string, token: string, count: number) {
    const snackbar = useSnackbar();
    if (!count) {
      snackbar.show({ text: '没有要标为已读的未读文章' });
      return;
    }
    snackbar.show({
      text: label.replace('{n}', String(count)),
      token,
      onUndone: async () => {
        await Promise.all([refreshLoaded(), refreshCounts()]);
      },
    });
  }

  /** 此篇及以上 / 以下标为已读：范围取自完整快照，含还没载入的部分（spec D7）。 */
  async function markRange(id: number, dir: 'above' | 'below') {
    const ids = snapshot.value.ids;
    const i = ids.indexOf(id);
    if (i < 0) return;
    const range = dir === 'above' ? ids.slice(0, i + 1) : ids.slice(i);
    await guarded('标为已读', async () => {
      const batch = await api.markItemsRead(range);
      markLocal(range, true);
      showBatch(
        `${dir === 'above' ? '此篇及以上' : '此篇及以下'} {n} 篇已标为已读`,
        batch.token,
        batch.count
      );
      await refreshCounts();
    });
  }

  /** 源、分类或全部订阅全部标为已读；当前视图以快照的 newest 为 ts，否则由后端取本地最新。 */
  async function markStream(s: string) {
    const ts = s === stream.value ? snapshot.value.newest : 0;
    await guarded('全部标为已读', async () => {
      const batch = await api.markStreamRead(s, ts);
      showBatch(`「${streamName(s)}」{n} 篇已标为已读`, batch.token, batch.count);
      await Promise.all([refreshLoaded(), refreshCounts()]);
    });
  }

  /** 一个同步周期结束：刷新侧栏与已载入卡片，把当前视图的新条目放进横幅。 */
  async function afterSync() {
    const my = epoch;
    const [s] = await Promise.all([
      api.snapshot(view.value, stream.value),
      refreshTree(),
      refreshCounts(),
      refreshLoaded(),
    ]);
    if (my !== epoch) return;
    const members = new Set(snapshot.value.ids);
    fresh.value = s.ids.filter((id) => !members.has(id));
  }

  /** 点横幅：重新取快照，选中的文章保持打开。 */
  async function loadNew() {
    await guarded('载入新文章', enter);
  }

  async function openInBrowser(id: number) {
    const url = cards.get(id)?.url;
    if (url) await guarded('在浏览器打开', () => api.openInBrowser(url));
  }

  return {
    view,
    stream,
    snapshot,
    cards,
    selectedId,
    tree,
    counts,
    loading,
    rows,
    hasMore,
    newCount,
    streamName,
    unreadIn,
    init,
    setView,
    setStream,
    loadMore,
    open,
    setRead,
    markRange,
    markStream,
    afterSync,
    loadNew,
    openInBrowser,
  };
});
