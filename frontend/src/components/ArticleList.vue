<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useNow } from '../composables/useNow';
import { useReader } from '../stores/reader';
import ArticleRow from './ArticleRow.vue';
import ContextMenu, { type MenuItem } from './ContextMenu.vue';

const reader = useReader();
const now = useNow();

const scroller = ref<HTMLElement | null>(null);
const sentinel = ref<HTMLElement | null>(null);
const menu = ref<{ x: number; y: number; id: number } | null>(null);

// 换了视图或载入新文章，列表回到顶部。
watch(
  () => reader.snapshot,
  () => scroller.value?.scrollTo?.({ top: 0 })
);

// 滚到底部附近时载入下一页卡片。
let observer: IntersectionObserver | undefined;
watch(sentinel, (el) => {
  observer?.disconnect();
  if (!el || typeof IntersectionObserver === 'undefined') return;
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((e) => e.isIntersecting)) void reader.loadMore();
    },
    { root: scroller.value, rootMargin: '600px' }
  );
  observer.observe(el);
});
onBeforeUnmount(() => observer?.disconnect());

function onMenu(e: MouseEvent, id: number) {
  menu.value = { x: e.clientX, y: e.clientY, id };
}

const menuItems = computed<MenuItem[]>(() => {
  const m = menu.value;
  if (!m) return [];
  const card = reader.cards.get(m.id);
  const ids = reader.snapshot.ids;
  const i = ids.indexOf(m.id);
  return [
    {
      label: card?.read ? '标为未读' : '标为已读',
      icon: card?.read ? 'mail' : 'mailOpen',
      run: () => void reader.setRead(m.id, !card?.read),
    },
    {
      label: '此篇及以上标为已读',
      icon: 'up',
      hint: `${i + 1} 篇`,
      sep: true,
      run: () => void reader.markRange(m.id, 'above'),
    },
    {
      label: '此篇及以下标为已读',
      icon: 'down',
      hint: `${ids.length - i} 篇`,
      run: () => void reader.markRange(m.id, 'below'),
    },
    {
      label: '在浏览器打开',
      icon: 'ext',
      sep: true,
      run: () => void reader.openInBrowser(m.id),
    },
  ];
});
</script>

<template>
  <section class="col list" aria-label="文章列表">
    <div class="scope-line">
      <b>{{ reader.streamName(reader.stream) }}</b> ·
      {{ reader.view === 'unread' ? '未读' : '全部' }} {{ reader.snapshot.ids.length }} 篇
    </div>
    <div ref="scroller" class="scroll" role="listbox" aria-label="文章">
      <button v-if="reader.newCount" class="new-banner" @click="reader.loadNew()">
        ↑ {{ reader.newCount }} 篇新文章，点击载入
      </button>
      <ArticleRow
        v-for="card in reader.rows"
        :key="card.id"
        :card="card"
        :selected="reader.selectedId === card.id"
        :now="now"
        @click="reader.open(card.id)"
        @contextmenu.prevent="onMenu($event, card.id)"
      />
      <div v-if="reader.hasMore" ref="sentinel" class="list-end">正在载入…</div>
      <div v-else-if="reader.snapshot.ids.length" class="list-end">
        — 已到底：共 {{ reader.snapshot.ids.length }} 篇 —
      </div>
      <div v-else-if="!reader.loading" class="empty">
        {{ reader.view === 'unread' ? '这里没有未读文章了' : '没有文章' }}
      </div>
    </div>

    <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menuItems" @close="menu = null" />
  </section>
</template>

<style scoped>
.list {
  width: 440px;
  flex: none;
  border-right: 1px solid var(--border);
  background: var(--bg);
}

.scope-line {
  flex: none;
  padding: 8px 16px 6px;
  font-size: 12px;
  color: var(--text-3);
}

.scope-line b {
  color: var(--text-2);
  font-weight: 600;
}

.new-banner {
  display: block;
  width: calc(100% - 24px);
  margin: 8px 12px 4px;
  padding: 7px 12px;
  border-radius: 16px;
  background: var(--accent);
  color: #fff;
  text-align: center;
  font-size: 13px;
  box-shadow: 0 2px 8px rgba(47, 111, 237, 0.3);
}

.new-banner:hover {
  filter: brightness(1.05);
}

.list-end {
  padding: 18px 16px 28px;
  text-align: center;
  color: var(--text-3);
  font-size: 12px;
}

.empty {
  padding: 60px 20px;
  text-align: center;
  color: var(--text-3);
}
</style>
