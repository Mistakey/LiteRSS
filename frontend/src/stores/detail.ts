/**
 * 详情栏：选中文章的正文、手动抓全文与摘要（spec D11）。
 *
 * 选中一篇就取它的 RSS 正文与已缓存的全文，有全文就显示全文，从不自动抓取。
 * 「抓取全文」由用户点：成功则替换显示，失败保留 RSS 正文并给中文原因；已有摘要时按全文重新生成。
 * 「摘要」由用户点，后端只用阅读区显示的正文（有缓存全文用全文，否则 RSS 正文）。
 * 「翻译」（spec D21）只对标题不是中文的文章：翻阅读区显示的正文，整篇译完才在对照与原文之间切换，
 * 切回对照不再请求；抓到全文后旧译文作废，回到原文。
 * 换文章后慢到的旧响应按序号丢弃。
 */
import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';
import { api } from '../api';
import { longEnoughToSummarize } from '../utils/article';
import { textBlocks } from '../utils/bilingual';
import { isChinese } from '../utils/format';
import { sanitizeArticleHtml } from '../utils/sanitize';
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
  /** 当前正文的译文，与 textBlocks(正文) 一一对应；null 为还没有。 */
  const translation = ref<string[] | null>(null);
  /** 显示对照；否则显示原文。 */
  const bilingual = ref(false);
  const translating = ref(false);
  const translateMessage = ref('');

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

  /** 标题判定为中文的文章没有「翻译」（未判定的仍给，spec D21 与 pitfall 12）。 */
  const translatable = computed(() => !!card.value && !isChinese(card.value));
  /** 正文已载入、不在抓全文：可以请求译文。块在点击时才从正文里取，不为没点翻译的文章解析正文。 */
  const canTranslate = computed(
    () => translatable.value && rss.value !== null && fetchStatus.value !== 'fetching'
  );
  /** 「翻译」按钮置灰：翻译中、对照中与已有译文时总能点（等待或切换），否则看 canTranslate。 */
  const translateDisabled = computed(
    () => !translating.value && !bilingual.value && !translation.value && !canTranslate.value
  );

  let epoch = 0;

  function resetTranslation() {
    translation.value = null;
    bilingual.value = false;
    translating.value = false;
    translateMessage.value = '';
  }

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
    resetTranslation();
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
        // 后端已删掉按 RSS 正文翻的译文；阅读区回到原文，要对照再点「翻译」。
        resetTranslation();
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

  /**
   * 「翻译」按钮：原文 → 对照（没有译文时先请求，整篇译完才切换）→ 原文 → 对照（用已有译文）。
   * 失败时停在原文并给中文原因，再点重试。
   */
  async function toggleTranslation() {
    const target = id.value;
    if (target === null || translating.value) return;
    if (bilingual.value) {
      bilingual.value = false;
      return;
    }
    if (translation.value) {
      bilingual.value = true;
      return;
    }
    if (!canTranslate.value) return;
    const my = epoch;
    const source = body.value;
    const sent = textBlocks(sanitizeArticleHtml(source));
    if (!sent.length) {
      translateMessage.value = '这篇文章没有可翻译的文字。';
      return;
    }
    translating.value = true;
    translateMessage.value = '';
    try {
      const tr = await api.translation(target, sent);
      if (my !== epoch || body.value !== source) return;
      if (tr.blocks.length === sent.length) {
        translation.value = tr.blocks;
        bilingual.value = true;
      } else {
        translateMessage.value = tr.message || '全文翻译失败，请稍后再试。';
      }
    } catch {
      if (my !== epoch || body.value !== source) return;
      translateMessage.value = '全文翻译失败，请稍后再试。';
    } finally {
      if (my === epoch && body.value === source) translating.value = false;
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
    translation,
    bilingual,
    translating,
    translateMessage,
    translatable,
    canTranslate,
    translateDisabled,
    fetchFullText,
    summarize,
    toggleTranslation,
    openLink,
  };
});
