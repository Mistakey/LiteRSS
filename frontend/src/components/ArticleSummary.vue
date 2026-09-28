<script setup lang="ts">
import { computed } from 'vue';
import { interceptLink } from '../utils/links';
import { sanitizeArticleHtml } from '../utils/sanitize';
import Icon from './Icon.vue';

// 摘要框：后端渲染的摘要 HTML 同样只经 sanitizeArticleHtml 进入 v-html（spec D16），
// 是两个豁免 vue/no-v-html 的组件之一。note 是后端给的中文说明（依据或没生成的原因）。
const props = defineProps<{ html: string; note: string; loading: boolean; waiting: boolean }>();
const emit = defineEmits<{ link: [url: string] }>();

const safe = computed(() => sanitizeArticleHtml(props.html));

// 摘要里的链接同正文：http(s) 交给系统浏览器，不在窗口里跳转。
function onClick(e: MouseEvent) {
  const url = interceptLink(e);
  if (url) emit('link', url);
}
</script>

<template>
  <section class="summary" :class="{ refused: !loading && !html }">
    <div class="h"><Icon name="spark" />AI 摘要</div>
    <template v-if="loading">
      <div class="wait">{{ waiting ? '等待全文抓取完成后生成摘要…' : '正在生成摘要…' }}</div>
      <div class="skeleton w1"></div>
      <div class="skeleton w2"></div>
      <div class="skeleton w3"></div>
    </template>
    <template v-else>
      <div v-if="html" class="content" @click="onClick" v-html="safe"></div>
      <div v-if="note" class="note">{{ note }}</div>
    </template>
  </section>
</template>

<style scoped>
.summary {
  border: 1px solid var(--border);
  border-left: 3px solid var(--accent);
  background: var(--bg-2);
  border-radius: 8px;
  padding: 12px 16px;
  margin: 0 0 22px;
  font: 15px/1.75 var(--read-font);
  color: var(--read-text);
}

.summary.refused {
  border-left-color: var(--warn-border);
}

.h {
  display: flex;
  align-items: center;
  gap: 6px;
  font: 600 13px/1.4 var(--ui-font);
  color: var(--accent);
  margin-bottom: 6px;
}

.h .icon {
  width: 16px;
  height: 16px;
}

.content :deep(ul),
.content :deep(ol) {
  margin: 0;
  padding-left: 1.2em;
}

.content :deep(p) {
  margin: 0 0 0.6em;
}

.content :deep(p:last-child) {
  margin-bottom: 0;
}

.content :deep(a) {
  color: var(--accent);
}

.note {
  margin-top: 8px;
  font: 12.5px/1.5 var(--ui-font);
  color: var(--warn-text);
}

.refused .note {
  margin-top: 0;
  font-size: 13.5px;
}

.wait {
  font-size: 13px;
  color: var(--text-3);
}

.skeleton {
  height: 12px;
  border-radius: 6px;
  background: linear-gradient(90deg, var(--bg-3), var(--bg-hover), var(--bg-3));
  background-size: 200% 100%;
  animation: shimmer 1.2s infinite;
  margin: 8px 0;
}

.w1 {
  width: 92%;
}

.w2 {
  width: 78%;
}

.w3 {
  width: 60%;
}

@keyframes shimmer {
  to {
    background-position: -200% 0;
  }
}
</style>
