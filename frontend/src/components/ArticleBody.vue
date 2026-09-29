<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { interleave } from '../utils/bilingual';
import { applyBionic } from '../utils/bionic';
import { interceptLink } from '../utils/links';
import { sanitizeArticleHtml } from '../utils/sanitize';

// 正文：不可信 HTML 只经 sanitizeArticleHtml 进入 v-html（spec D16），这是两个豁免 vue/no-v-html 的组件之一。
// 排版写死（spec D15）。点图打开查看器；点 http(s) 链接交给系统浏览器，不在窗口里跳转。
// 给了译文时显示中英对照（spec D21）：interleave 在清洗结果里插入纯文本译文，插入后的 HTML 再清洗一次才进 v-html，
// 免得「清洗 → 解析 → 序列化 → 再解析」这一轮给变异型 XSS 留口子。
// bionic 为真时在公式、代码增强之后加粗英文词首（spec D22）；开关一变就以新 key 重建正文节点，每份 DOM 只加粗一次。
const props = defineProps<{ html: string; translation?: string[] | null; bionic?: boolean }>();
const emit = defineEmits<{ image: [images: string[], index: number]; link: [url: string] }>();

const root = ref<HTMLElement>();
/** 每次正文重建加一；异步增强回来时不是最新一轮就作废。 */
let run = 0;
const safe = computed(() => {
  const clean = sanitizeArticleHtml(props.html);
  return props.translation ? sanitizeArticleHtml(interleave(clean, props.translation)) : clean;
});

watch(
  [safe, () => props.bionic],
  async () => {
    const my = ++run;
    await nextTick();
    const el = root.value;
    if (!el || my !== run) return;
    // 只有含公式或代码的正文才载入 KaTeX 与 highlight.js。
    if (/[$\\]|<pre|math/.test(safe.value)) {
      const { enhance } = await import('../utils/enhance');
      // 载入期间正文已重建：交给新的一轮，免得同一份 DOM 增强两次。
      if (my !== run || root.value !== el) return;
      enhance(el);
    }
    if (props.bionic) applyBionic(el);
  },
  { immediate: true }
);

function onClick(e: MouseEvent) {
  const target = e.target as Element;
  const img = target.closest('img');
  if (img && root.value) {
    e.preventDefault();
    const imgs = [...root.value.querySelectorAll('img')].filter((i) => i.src);
    const index = imgs.indexOf(img);
    if (index >= 0)
      emit(
        'image',
        imgs.map((i) => i.src),
        index
      );
    return;
  }
  const url = interceptLink(e);
  if (url) emit('link', url);
}
</script>

<template>
  <div
    ref="root"
    :key="bionic ? 'bionic' : 'plain'"
    class="article-body"
    @click="onClick"
    v-html="safe"
  ></div>
</template>

<style scoped>
.article-body {
  font-family: var(--read-font);
  font-size: var(--read-size);
  line-height: var(--read-lh);
  color: var(--read-text);
  max-width: var(--read-width);
  overflow-wrap: break-word;
}

.article-body :deep(p) {
  margin: 0 0 1em;
}

.article-body :deep(h1),
.article-body :deep(h2),
.article-body :deep(h3),
.article-body :deep(h4) {
  font-size: 1.2em;
  line-height: 1.5;
  margin: 1.6em 0 0.6em;
}

.article-body :deep(h1) {
  font-size: 1.35em;
}

.article-body :deep(ul),
.article-body :deep(ol) {
  padding-left: 1.4em;
  margin: 0 0 1em;
}

.article-body :deep(figure) {
  margin: 1.4em 0;
}

.article-body :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 6px;
  cursor: zoom-in;
}

.article-body :deep(figcaption) {
  font-size: 13px;
  color: var(--text-3);
  text-align: center;
  margin-top: 6px;
  line-height: 1.5;
}

.article-body :deep(a) {
  color: var(--accent);
}

/* 对照里的译文：紧跟原文，淡一些并带左侧细线（spec D21） */
.article-body :deep(.literss-tr) {
  display: block;
  margin-top: 0.35em;
  padding-left: 0.75em;
  border-left: 2px solid var(--border);
  color: var(--text-2);
  font-weight: normal;
  text-align: start;
}

.article-body :deep(a.embed-link) {
  display: inline-block;
  margin: 0.4em 0;
  padding: 4px 12px;
  border: 1px solid var(--border);
  border-radius: 7px;
  text-decoration: none;
  font-size: 14px;
}

.article-body :deep(blockquote) {
  margin: 0 0 1em;
  padding: 0 1em;
  border-left: 3px solid var(--border);
  color: var(--text-2);
}

.article-body :deep(pre) {
  overflow-x: auto;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--bg-2);
  font-size: 14px;
  line-height: 1.6;
}

.article-body :deep(code) {
  font-family: Consolas, 'SF Mono', Menlo, monospace;
  font-size: 0.9em;
}

.article-body :deep(:not(pre) > code) {
  padding: 0.1em 0.35em;
  border-radius: 4px;
  background: var(--bg-3);
}

.article-body :deep(table) {
  display: block;
  overflow-x: auto;
  border-collapse: collapse;
  margin: 0 0 1em;
  font-size: 15px;
}

.article-body :deep(th),
.article-body :deep(td) {
  border: 1px solid var(--border);
  padding: 6px 10px;
}

.article-body :deep(hr) {
  border: 0;
  border-top: 1px solid var(--border);
  margin: 1.6em 0;
}

/* Bionic Reading 的词首（spec D22） */
.article-body :deep(.bionic-fix) {
  font-weight: 700;
}

/* highlight.js 配色，跟随系统主题 */
.article-body :deep(.hljs-comment),
.article-body :deep(.hljs-quote) {
  color: var(--text-3);
  font-style: italic;
}

.article-body :deep(.hljs-keyword),
.article-body :deep(.hljs-selector-tag),
.article-body :deep(.hljs-built_in) {
  color: var(--code-keyword);
}

.article-body :deep(.hljs-string),
.article-body :deep(.hljs-attr),
.article-body :deep(.hljs-regexp) {
  color: var(--code-string);
}

.article-body :deep(.hljs-number),
.article-body :deep(.hljs-literal),
.article-body :deep(.hljs-variable) {
  color: var(--code-number);
}

.article-body :deep(.hljs-title),
.article-body :deep(.hljs-section),
.article-body :deep(.hljs-name) {
  color: var(--code-title);
}
</style>
