<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { isLabel, READING_LIST } from '../api';
import { useReader } from '../stores/reader';
import ContextMenu, { type MenuItem } from './ContextMenu.vue';
import FeedBadge from './FeedBadge.vue';
import Icon from './Icon.vue';

const reader = useReader();

const collapsed = reactive(new Set<string>());
const menu = ref<{ x: number; y: number; stream: string } | null>(null);

function toggle(id: string) {
  if (collapsed.has(id)) collapsed.delete(id);
  else collapsed.add(id);
}

function onMenu(e: MouseEvent, stream: string) {
  menu.value = { x: e.clientX, y: e.clientY, stream };
}

const menuItems = computed<MenuItem[]>(() => {
  const m = menu.value;
  if (!m) return [];
  const n = reader.unreadIn(m.stream);
  return [
    {
      label: '全部标为已读',
      icon: 'checks',
      hint: `${n} 篇`,
      disabled: !n,
      run: () => void reader.markStream(m.stream),
    },
  ];
});

const menuCaption = computed(() => {
  const s = menu.value?.stream;
  if (!s) return '';
  const cat = isLabel(s) ? '（含其下所有源）' : '';
  return reader.streamName(s) + cat;
});
</script>

<template>
  <aside class="col sidebar" aria-label="订阅">
    <div class="seg" role="group" aria-label="视图">
      <button
        :class="{ on: reader.view === 'unread' }"
        :aria-pressed="reader.view === 'unread'"
        @click="reader.setView('unread')"
      >
        未读 <span class="n">{{ reader.counts.total }}</span>
      </button>
      <button
        :class="{ on: reader.view === 'all' }"
        :aria-pressed="reader.view === 'all'"
        @click="reader.setView('all')"
      >
        全部
      </button>
    </div>

    <nav class="scroll tree">
      <div
        class="node top"
        :class="{ on: reader.stream === READING_LIST, dim: !reader.counts.total }"
        @click="reader.setStream(READING_LIST)"
        @contextmenu.prevent="onMenu($event, READING_LIST)"
      >
        <Icon name="all" /><span class="name">全部订阅</span>
        <span v-if="reader.counts.total" class="cnt">{{ reader.counts.total }}</span>
      </div>
      <div class="sb-sep"></div>

      <template v-for="cat in reader.sidebarTree.categories" :key="cat.id">
        <div
          class="node cat"
          :class="{ on: reader.stream === cat.id, dim: !reader.unreadIn(cat.id) }"
          @click="reader.setStream(cat.id)"
          @contextmenu.prevent="onMenu($event, cat.id)"
        >
          <button
            class="chev-btn"
            :aria-label="collapsed.has(cat.id) ? '展开' : '折叠'"
            :aria-expanded="!collapsed.has(cat.id)"
            @click.stop="toggle(cat.id)"
          >
            <Icon name="chev" class="chev" :class="{ closed: collapsed.has(cat.id) }" />
          </button>
          <span class="name">{{ cat.label }}</span>
          <span v-if="reader.unreadIn(cat.id)" class="cnt">{{ reader.unreadIn(cat.id) }}</span>
        </div>
        <template v-if="!collapsed.has(cat.id)">
          <div
            v-for="f in cat.feeds"
            :key="cat.id + f.id"
            class="node feed"
            :class="{ on: reader.stream === f.id, dim: !reader.unreadIn(f.id) }"
            @click="reader.setStream(f.id)"
            @contextmenu.prevent="onMenu($event, f.id)"
          >
            <FeedBadge :id="f.id" :title="f.title" /><span class="name">{{ f.title }}</span>
            <span v-if="reader.unreadIn(f.id)" class="cnt">{{ reader.unreadIn(f.id) }}</span>
          </div>
        </template>
      </template>

      <div
        v-for="f in reader.sidebarTree.feeds"
        :key="f.id"
        class="node feed top"
        :class="{ on: reader.stream === f.id, dim: !reader.unreadIn(f.id) }"
        @click="reader.setStream(f.id)"
        @contextmenu.prevent="onMenu($event, f.id)"
      >
        <FeedBadge :id="f.id" :title="f.title" /><span class="name">{{ f.title }}</span>
        <span v-if="reader.unreadIn(f.id)" class="cnt">{{ reader.unreadIn(f.id) }}</span>
      </div>
    </nav>

    <ContextMenu
      v-if="menu"
      :x="menu.x"
      :y="menu.y"
      :caption="menuCaption"
      :items="menuItems"
      @close="menu = null"
    />
  </aside>
</template>

<style scoped>
.sidebar {
  width: 248px;
  flex: none;
  background: var(--bg-2);
  border-right: 1px solid var(--border);
}

.seg {
  display: flex;
  margin: 10px 10px 6px;
  padding: 3px;
  background: var(--bg-3);
  border-radius: 9px;
  flex: none;
}

.seg button {
  flex: 1;
  height: 30px;
  border-radius: 7px;
  color: var(--text-2);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-weight: 500;
}

.seg button.on {
  background: var(--bg);
  color: var(--text);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
}

.seg .n {
  font-size: 11px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}

.tree {
  padding: 4px 6px 16px;
}

.node {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 30px;
  padding: 0 8px;
  border-radius: 6px;
  cursor: pointer;
  user-select: none;
}

.node:hover {
  background: var(--bg-hover);
}

.node.on {
  background: var(--bg-sel);
  color: var(--accent);
}

.node .name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node .cnt {
  font-size: 12px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}

.node.on .cnt {
  color: var(--accent);
}

.node.dim .name {
  color: var(--text-3);
}

.node.feed {
  padding-left: 26px;
}

.node.feed.top {
  padding-left: 8px;
}

.node.cat .name {
  font-weight: 600;
}

.chev-btn {
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  margin-left: -2px;
}

.chev {
  width: 14px;
  height: 14px;
  color: var(--text-3);
  transition: transform 0.12s;
}

.chev.closed {
  transform: rotate(-90deg);
}

.sb-sep {
  height: 1px;
  background: var(--border);
  margin: 6px 8px;
}
</style>
