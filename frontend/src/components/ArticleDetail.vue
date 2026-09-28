<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useDetail } from '../stores/detail';
import { useReader } from '../stores/reader';
import { useNow } from '../composables/useNow';
import { displayTitle, isTranslated, relativeTime } from '../utils/format';
import ArticleBody from './ArticleBody.vue';
import ArticleSummary from './ArticleSummary.vue';
import FeedBadge from './FeedBadge.vue';
import Icon from './Icon.vue';
import ImageViewer from './ImageViewer.vue';

// 详情栏（spec D15）：无工具栏；标题块、抓全文提示、摘要框、正文；「摘要 / 在浏览器打开 / 标为未读」在底部浮动条。
const reader = useReader();
const detail = useDetail();
const now = useNow();

const scroller = ref<HTMLElement>();
const viewer = ref<{ images: string[]; start: number } | null>(null);

const card = computed(() => detail.card);
const showSummary = computed(
  () => detail.summaryLoading || !!detail.summaryHtml || !!detail.summaryNote
);
const summaryLabel = computed(() =>
  detail.summaryLoading ? '生成中…' : detail.summaryHtml ? '已摘要' : '摘要'
);
/** 抓取失败且 RSS 正文太短：提示里的「在浏览器打开」是主按钮。 */
const shortFailure = computed(() => detail.fetchStatus === 'failed' && !detail.canSummarize);

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
            <div class="grow">RSS 里的正文被截断，正在抓取全文…</div>
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

          <ArticleSummary
            v-if="showSummary"
            :html="detail.summaryHtml"
            :note="detail.summaryNote"
            :loading="detail.summaryLoading"
            :waiting="detail.fetchStatus === 'fetching'"
            @link="detail.openLink"
          />

          <div v-if="detail.rss === null" class="loading">正在载入正文…</div>
          <ArticleBody
            v-else
            :html="detail.body"
            @image="(images, start) => (viewer = { images, start })"
            @link="detail.openLink"
          />
          <p v-if="detail.rss !== null && !detail.body && !detail.loadError" class="empty">
            这篇文章没有正文。
          </p>
        </article>
      </div>

      <div class="float-bar">
        <button
          class="btn primary"
          :disabled="!detail.canSummarize || detail.summaryLoading || !!detail.summaryHtml"
          :title="detail.canSummarize ? undefined : '正文太短，无法摘要'"
          @click="detail.summarize"
        >
          <Icon name="spark" />{{ summaryLabel }}
        </button>
        <button class="btn" @click="openInBrowser"><Icon name="ext" />在浏览器打开</button>
        <span class="div"></span>
        <button class="btn ghost" @click="toggleRead">
          <Icon :name="card.read ? 'mail' : 'mailOpen'" />{{ card.read ? '标为未读' : '标为已读' }}
        </button>
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

.btn.ghost {
  border-color: transparent;
  color: var(--text-2);
}

.btn.ghost:hover {
  color: var(--text);
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

.float-bar .div {
  width: 1px;
  background: var(--border);
  margin: 6px 2px;
}
</style>
