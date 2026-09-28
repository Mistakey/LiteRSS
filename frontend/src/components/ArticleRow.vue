<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Card } from '../api';
import { displayTitle, isChinese, isTranslated, relativeTime } from '../utils/format';
import { safeImageUrl } from '../utils/sanitize';
import FeedBadge from './FeedBadge.vue';

// 宽松行（变体 C 样式，spec D15）：源与时间 → 标题最多两行 → 一行中文摘录，有配图才显示右侧缩略图。
const props = defineProps<{ card: Card; selected: boolean; now: number }>();

const title = computed(() => displayTitle(props.card));
const translated = computed(() => isTranslated(props.card));
const excerpt = computed(() => (isChinese(props.card) ? props.card.excerpt : ''));
// image_url 取自不可信正文，同样不能指向应用自身源或回环主机（spec D16）。
const thumb = computed(() => safeImageUrl(props.card.image_url));
const thumbFailed = ref(false);
</script>

<template>
  <div
    class="row"
    :class="{ read: card.read, on: selected }"
    role="option"
    :aria-selected="selected"
    :title="translated ? card.title : undefined"
  >
    <span class="dot"></span>
    <div class="body">
      <div class="meta">
        <FeedBadge :id="card.stream_id" :title="card.feed_title || '?'" />
        <span class="src">{{ card.feed_title || '未知订阅源' }}</span>
        · {{ relativeTime(card.published_at, now) }}
        <span v-if="translated" class="chip">译</span>
      </div>
      <div class="t">{{ title }}</div>
      <div v-if="excerpt" class="sn">{{ excerpt }}</div>
    </div>
    <img
      v-if="thumb && !thumbFailed"
      class="thumb"
      :src="thumb"
      alt=""
      loading="lazy"
      @error="thumbFailed = true"
    />
  </div>
</template>

<style scoped>
.row {
  position: relative;
  cursor: pointer;
  border-bottom: 1px solid var(--border);
  padding: 12px 14px 12px 16px;
  display: flex;
  gap: 12px;
}

.row:hover {
  background: var(--bg-hover);
}

.row.on {
  background: var(--bg-sel);
}

.dot {
  position: absolute;
  left: 6px;
  top: 20px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent);
}

.row.read .dot {
  background: transparent;
}

.body {
  flex: 1;
  min-width: 0;
}

.meta {
  font-size: 12px;
  color: var(--text-3);
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 3px;
  white-space: nowrap;
}

.src {
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 180px;
}

.t {
  font-weight: 600;
  font-size: 15px;
  line-height: 1.45;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.row.read .t {
  color: var(--text-3);
  font-weight: 400;
}

.sn {
  margin-top: 3px;
  font-size: 13px;
  color: var(--text-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.row.read .sn {
  color: var(--text-3);
}

.thumb {
  width: 72px;
  height: 54px;
  border-radius: 6px;
  object-fit: cover;
  flex: none;
  background: var(--bg-3);
}

.chip {
  font-size: 11px;
  padding: 0 5px;
  border-radius: 4px;
  background: var(--bg-3);
  color: var(--text-2);
  line-height: 17px;
}
</style>
