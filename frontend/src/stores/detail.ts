/**
 * 详情栏：选中文章的正文、手动抓全文与摘要（spec D11）。
 *
 * 选中一篇就取它的 RSS 正文与已缓存的全文，有全文就显示全文，从不自动抓取。
 * 「抓取全文」由用户点：成功则替换显示，失败保留 RSS 正文并给中文原因；已有摘要时按全文重新生成。
 * 「摘要」由用户点，后端只用阅读区显示的正文（有缓存全文用全文，否则 RSS 正文）。
 * 换文章后慢到的旧响应按序号丢弃。
 */
import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';
import { api } from '../api';
import { longEnoughToSummarize } from '../utils/article';
import { useReader } from './reader';
import { useSnackbar } from './snackbar';

/** none：显示 RSS 正文；fetching、ok、failed：抓全文的进展，ok 也包括打开时已缓存。 */
export type FetchStatus = 'none' | 'fetching' | 'ok' | 'failed';

export const useDetail = defineStore('detail', () => {
  const reader = useReader();

  const id = ref<number | null>(null);
  /** RSS 正文；null 表示还在载入。 */
  const rss = ref<string | null>(null);
  const loadError = ref('');
  const fetchStatus = ref<FetchStatus>('none');
  const fullText = ref('');
  const fetchMessage = ref('');
  const summaryLoading = ref(false);
  const summaryHtml = ref('');
  const summaryNote = ref('');

  const card = computed(() => (id.value === null ? undefined : reader.cards.get(id.value)));
  /** 显示的正文：有全文就用全文。 */
  const body = computed(() => (fetchStatus.value === 'ok' ? fullText.value : (rss.value ?? '')));
  /** 显示的正文够长才能摘要（与后端门槛一致）；抓取中不摘要，免得先按 RSS 生成。 */
  const canSummarize = computed(
    () => fetchStatus.value !== 'fetching' && longEnoughToSummarize(body.value)
  );
  /** RSS 正文太短、还没有全文：提示可以先抓全文。 */
  const suggestFullText = computed(
    () =>
      rss.value !== null &&
      !loadError.value &&
      fetchStatus.value === 'none' &&
      !longEnoughToSummarize(rss.value)
  );

  let epoch = 0;

  async function show(next: number | null) {
    const my = ++epoch;
    id.value = next;
    rss.value = null;
    loadError.value = '';
    fetchStatus.value = 'none';
    fullText.value = '';
    fetchMessage.value = '';
    summaryLoading.value = false;
    summaryHtml.value = '';
    summaryNote.value = '';
    if (next === null) return;

    try {
      const b = await api.content(next);
      if (my !== epoch) return;
      rss.value = b.content;
      if (b.fulltext) {
        fullText.value = b.fulltext;
        fetchStatus.value = 'ok';
      }
    } catch {
      if (my !== epoch) return;
      rss.value = '';
      loadError.value = '正文载入失败，请稍后重试。';
    }
  }

  /** 抓全文；成功后已有的摘要（基于 RSS 正文）按全文重新生成，后端已丢掉旧的。 */
  async function fetchFullText() {
    const target = id.value;
    if (target === null || fetchStatus.value === 'fetching' || fetchStatus.value === 'ok') return;
    const my = epoch;
    fetchStatus.value = 'fetching';
    fetchMessage.value = '';
    try {
      const ft = await api.fullText(target);
      if (my !== epoch) return;
      if (ft.outcome === 'success') {
        fullText.value = ft.content;
        fetchStatus.value = 'ok';
        if (summaryHtml.value || summaryNote.value) {
          summaryHtml.value = '';
          summaryNote.value = '';
          await summarize();
        }
      } else {
        fetchMessage.value = ft.message || '未能获取全文。';
        fetchStatus.value = 'failed';
      }
    } catch {
      if (my !== epoch) return;
      fetchMessage.value = '抓取全文时出错，请稍后重试。';
      fetchStatus.value = 'failed';
    }
  }

  /** 生成或取回摘要；没生成时 note 说明原因，按钮仍可再点（例如配好模型后）。 */
  async function summarize() {
    const target = id.value;
    if (target === null || summaryLoading.value || summaryHtml.value || !canSummarize.value) return;
    const my = epoch;
    summaryLoading.value = true;
    summaryNote.value = '';
    try {
      const s = await api.summary(target);
      if (my !== epoch) return;
      summaryHtml.value = s.html;
      summaryNote.value = s.note;
    } catch {
      if (my !== epoch) return;
      summaryNote.value = '摘要生成失败，请稍后再试。';
    } finally {
      if (my === epoch) summaryLoading.value = false;
    }
  }

  /** 正文里的链接与嵌入内容外链：交给系统浏览器。 */
  async function openLink(url: string) {
    try {
      await api.openInBrowser(url);
    } catch {
      useSnackbar().show({ text: '在浏览器打开失败，请稍后重试' });
    }
  }

  watch(
    () => reader.selectedId,
    (next) => void show(next),
    { immediate: true }
  );

  return {
    id,
    card,
    rss,
    loadError,
    fetchStatus,
    fetchMessage,
    body,
    canSummarize,
    suggestFullText,
    summaryLoading,
    summaryHtml,
    summaryNote,
    fetchFullText,
    summarize,
    openLink,
  };
});
