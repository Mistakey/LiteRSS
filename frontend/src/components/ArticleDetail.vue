<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useDetail } from '../stores/detail';
import { usePrefs } from '../stores/prefs';
import { useReader } from '../stores/reader';
import { useNow } from '../composables/useNow';
import { displayTitle, isTranslated, relativeTime } from '../utils/format';
import ArticleBody from './ArticleBody.vue';
import ArticleSummary from './ArticleSummary.vue';
import FeedBadge from './FeedBadge.vue';
import Icon from './Icon.vue';
import type { IconName } from './icons';
import ImageViewer from './ImageViewer.vue';

// 详情栏（spec D15）：无工具栏；标题块、抓全文提示、摘要框、正文。
// 底部浮动条只有图标：摘要、翻译、Bionic Reading、抓取全文、在浏览器打开，分隔线后是已读。
// 图标表示当前状态，高亮底色表示开着，进行中换成转圈；文字进悬停提示与无障碍名称，写点击后会发生什么。
// 已显示全文时没有「抓取全文」，标题是中文的文章没有「翻译」（spec D21）。
const reader = useReader();
const detail = useDetail();
const prefs = usePrefs();
const now = useNow();

const scroller = ref<HTMLElement>();
const viewer = ref<{ images: string[]; start: number } | null>(null);

const card = computed(() => detail.card);
const showSummary = computed(
  () => detail.summaryLoading || !!detail.summaryHtml || !!detail.summaryNote
);
/** 抓取失败且 RSS 正文太短：提示里的「在浏览器打开」是主按钮。 */
const shortFailure = computed(() => detail.fetchStatus === 'failed' && !detail.canSummarize);

type BarKey = 'summary' | 'translate' | 'bionic' | 'fetch' | 'ext' | 'read';

interface BarButton {
  key: BarKey;
  icon: IconName;
  /** 悬停提示与无障碍名称。 */
  label: string;
  onClick: () => void;
  spin?: boolean;
  primary?: boolean;
  /** 开着：高亮底色。 */
  active?: boolean;
  /** 已完成、不能再点，但不置灰。 */
  done?: boolean;
  /** 开关类按钮的 aria-pressed；其余不设。 */
  pressed?: boolean;
  disabled?: boolean;
}

const busy = (key: BarKey, label: string, rest: Partial<BarButton> = {}): BarButton => ({
  key,
  icon: 'refresh',
  label,
  spin: true,
  disabled: true,
  onClick: () => {},
  ...rest,
});

function summaryButton(): BarButton {
  const base = { key: 'summary', onClick: detail.summarize } as const;
  if (detail.summaryLoading) return busy('summary', '摘要生成中…', { primary: true });
  if (detail.summaryHtml)
    return { ...base, icon: 'sparkCheck', label: '已生成摘要', done: true, disabled: true };
  if (detail.canSummarize) return { ...base, icon: 'spark', label: '生成摘要', primary: true };
  let label = '正文太短，可以先抓取全文';
  if (detail.fetchStatus === 'fetching') label = '全文抓取中，稍后可以生成摘要';
  else if (detail.fetchStatus === 'ok') label = '正文太短，无法摘要';
  return { ...base, icon: 'spark', label, primary: true, disabled: true };
}

function translateButton(): BarButton {
  const base = { key: 'translate', onClick: detail.toggleTranslation } as const;
  if (detail.translating) return busy('translate', '翻译中…', { pressed: false });
  if (detail.bilingual)
    return {
      ...base,
      icon: 'bilingual',
      label: '正在对照，点击回到原文',
      active: true,
      pressed: true,
    };
  if (detail.translateDisabled) {
    const label = detail.fetchStatus === 'fetching' ? '全文抓取中，稍后可以翻译' : '正文载入中';
    return { ...base, icon: 'translate', label, pressed: false, disabled: true };
  }
  return { ...base, icon: 'translate', label: '翻译：中英段落对照', pressed: false };
}

function fetchButton(): BarButton {
  if (detail.fetchStatus === 'fetching') return busy('fetch', '正在抓取全文…');
  const loading = detail.rss === null;
  return {
    key: 'fetch',
    icon: 'cloud',
    label: loading ? '正文载入中' : '抓取全文',
    disabled: loading,
    onClick: detail.fetchFullText,
  };
}

const buttons = computed<BarButton[]>(() => {
  const out = [summaryButton()];
  if (detail.translatable) out.push(translateButton());
  out.push({
    key: 'bionic',
    icon: prefs.bionic ? 'bionic' : 'bionicOff',
    label: prefs.bionic ? 'Bionic Reading 已开，点击关闭' : 'Bionic Reading：加粗英文词首',
    active: prefs.bionic,
    pressed: prefs.bionic,
    onClick: prefs.toggleBionic,
  });
  if (detail.fetchStatus !== 'ok') out.push(fetchButton());
  out.push({ key: 'ext', icon: 'ext', label: '在浏览器打开', onClick: openInBrowser });
  const read = !!card.value?.read;
  out.push({
    key: 'read',
    icon: read ? 'mailOpen' : 'mail',
    label: read ? '已读，点击标为未读' : '未读，点击标为已读',
    onClick: toggleRead,
  });
  return out;
});

watch(
  () => detail.id,
  async () => {
    viewer.value = null;
    await nextTick();
    if (scroller.value) scroller.value.scrollTop = 0;
  }
);

function openInBrowser() {
  if (detail.id !== null) void reader.openInBrowser(detail.id);
}

function toggleRead() {
  if (card.value) void reader.setRead(card.value.id, !card.value.read);
}
</script>

<template>
  <main class="col detail">
    <template v-if="card">
      <div ref="scroller" class="scroll">
        <article class="reader">
          <div class="src-line">
            <FeedBadge :id="card.stream_id" :title="card.feed_title || '?'" />
            <span>{{ card.feed_title || '未知订阅源' }}</span>
            <span>· {{ relativeTime(card.published_at, now) }}</span>
          </div>
          <h1>{{ displayTitle(card) }}</h1>
          <div v-if="isTranslated(card)" class="orig">{{ card.title }}</div>
          <hr />

          <div v-if="detail.loadError" class="notice warn">
            <Icon name="warn" />
            <div class="grow">{{ detail.loadError }}</div>
          </div>
          <div v-if="detail.fetchStatus === 'fetching'" class="notice info">
            <Icon name="refresh" class="spin" />
            <div class="grow">正在抓取全文…</div>
          </div>
          <div v-else-if="detail.suggestFullText" class="notice info">
            <Icon name="cloud" />
            <div class="grow">RSS 只提供了很少的内容，可以先抓取全文再生成摘要。</div>
            <button class="btn primary" @click="detail.fetchFullText">
              <Icon name="cloud" />抓取全文
            </button>
          </div>
          <div v-else-if="detail.fetchStatus === 'failed'" class="notice warn">
            <Icon name="warn" />
            <div class="grow">
              {{ detail.fetchMessage }}
              {{
                shortFailure
                  ? 'RSS 只提供了很少的内容，无法生成摘要。'
                  : '下面显示的是 RSS 提供的正文。'
              }}
            </div>
            <button class="btn" :class="{ primary: shortFailure }" @click="openInBrowser">
              <Icon name="ext" />在浏览器打开
            </button>
          </div>

          <div v-if="detail.translateMessage" class="notice warn translate-failed">
            <Icon name="warn" />
            <div class="grow">{{ detail.translateMessage }}</div>
          </div>

          <ArticleSummary
            v-if="showSummary"
            :html="detail.summaryHtml"
            :note="detail.summaryNote"
            :loading="detail.summaryLoading"
            @link="detail.openLink"
          />

          <div v-if="detail.rss === null" class="loading">正在载入正文…</div>
          <ArticleBody
            v-else
            :html="detail.body"
            :translation="detail.bilingual ? detail.translation : null"
            :bionic="prefs.bionic"
            @image="(images, start) => (viewer = { images, start })"
            @link="detail.openLink"
          />
          <p v-if="detail.rss !== null && !detail.body && !detail.loadError" class="empty">
            这篇文章没有正文。
          </p>
        </article>
      </div>

      <div class="float-bar">
        <template v-for="b in buttons" :key="b.key">
          <span v-if="b.key === 'read'" class="div"></span>
          <button
            class="btn square"
            :class="[b.key, { primary: b.primary, active: b.active, done: b.done }]"
            :disabled="b.disabled"
            :title="b.label"
            :aria-label="b.label"
            :aria-pressed="b.pressed"
            @click="b.onClick"
          >
            <Icon :name="b.icon" :class="{ spin: b.spin }" />
          </button>
        </template>
      </div>
    </template>
    <div v-else class="placeholder">从列表选一篇文章</div>

    <ImageViewer
      v-if="viewer"
      :images="viewer.images"
      :start="viewer.start"
      @close="viewer = null"
    />
  </main>
</template>

<style scoped>
.detail {
  flex: 1;
  background: var(--bg);
  position: relative;
}

.placeholder {
  height: 100%;
  display: grid;
  place-items: center;
  color: var(--text-3);
  text-align: center;
}

.reader {
  max-width: calc(var(--read-width) + 64px);
  margin: 0 auto;
  padding: 28px 32px 120px;
}

.src-line {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  color: var(--text-2);
  margin-bottom: 10px;
}

h1 {
  font: 700 var(--read-title-size) / 1.4 var(--read-font);
  margin: 0 0 6px;
  letter-spacing: 0.01em;
}

.orig {
  font: 15px/1.5 var(--read-font);
  color: var(--text-3);
  margin: 0 0 10px;
}

hr {
  border: 0;
  border-top: 1px solid var(--border);
  margin: 18px 0 22px;
}

.notice {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px 14px;
  border-radius: 8px;
  margin: 0 0 18px;
  font-size: 13.5px;
  line-height: 1.6;
}

.notice .icon {
  margin-top: 2px;
}

.notice.info {
  background: var(--accent-soft);
  color: var(--text);
}

.notice.warn {
  background: var(--warn-bg);
  color: var(--warn-text);
  border: 1px solid var(--warn-border);
}

.notice .btn {
  height: 28px;
  font-size: 13px;
}

.loading,
.empty {
  color: var(--text-3);
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.btn {
  height: 32px;
  padding: 0 12px;
  border-radius: 7px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--text);
  border: 1px solid var(--border);
  background: var(--bg);
  white-space: nowrap;
}

.btn:hover {
  background: var(--bg-hover);
}

.btn.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
}

.btn.primary:hover {
  filter: brightness(1.06);
}

.btn[disabled] {
  opacity: 0.45;
  cursor: not-allowed;
}

.btn .icon {
  width: 16px;
  height: 16px;
}

.float-bar {
  position: absolute;
  left: 50%;
  bottom: 22px;
  transform: translateX(-50%);
  display: flex;
  gap: 4px;
  padding: 5px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: var(--shadow);
  z-index: 5;
}

.float-bar .btn {
  border-color: transparent;
  height: 36px;
}

.float-bar .btn.primary {
  border-color: var(--accent);
}

.float-bar .btn.square {
  width: 36px;
  padding: 0;
  justify-content: center;
}

.float-bar .btn.read {
  color: var(--text-2);
}

.float-bar .btn.read:hover {
  color: var(--text);
}

.float-bar .btn.active {
  background: var(--accent-soft);
  color: var(--accent);
}

.float-bar .btn.done[disabled] {
  opacity: 1;
  cursor: default;
  color: var(--accent);
}

.float-bar .div {
  width: 1px;
  background: var(--border);
  margin: 6px 2px;
}
</style>
