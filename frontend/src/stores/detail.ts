/**
 * 详情栏：选中文章的正文、自动抓全文与摘要（spec D11）。
 *
 * 选中一篇就取它的 RSS 正文；看起来被截断时自动抓全文，成功则替换显示，失败保留 RSS 正文并给中文原因。
 * 「摘要」由用户点；后端总是先用全文（与正在进行的抓取合并），全文抓不到时 RSS 正文够长才摘要。
 * 换文章后慢到的旧响应按序号丢弃。
 */
import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';
import { api } from '../api';
import { isTruncated, longEnoughToSummarize } from '../utils/article';
import { useReader } from './reader';
import { useSnackbar } from './snackbar';

/** none：不需要抓（正文完整）；fetching、ok、failed：自动抓全文的进展。 */
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
  /** 显示的正文：抓到全文就用全文。 */
  const body = computed(() => (fetchStatus.value === 'ok' ? fullText.value : (rss.value ?? '')));
  /** 全文抓不到且 RSS 正文太短时不能摘要（与后端门槛一致）。 */
  const canSummarize = computed(
    () => !(fetchStatus.value === 'failed' && !longEnoughToSummarize(rss.value ?? ''))
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

    let content: string;
    try {
      content = await api.content(next);
    } catch {
      if (my !== epoch) return;
      rss.value = '';
      loadError.value = '正文载入失败，请稍后重试。';
      return;
    }
    if (my !== epoch) return;
    rss.value = content;
    if (isTruncated(content)) await fetchFullText(my, next);
  }

  async function fetchFullText(my: number, target: number) {
    fetchStatus.value = 'fetching';
    try {
      const ft = await api.fullText(target);
      if (my !== epoch) return;
      if (ft.outcome === 'success') {
        fullText.value = ft.content;
        fetchStatus.value = 'ok';
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
    summaryLoading,
    summaryHtml,
    summaryNote,
    summarize,
    openLink,
  };
});
