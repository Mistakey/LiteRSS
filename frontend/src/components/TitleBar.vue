<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { api } from '../api';
import { useNow } from '../composables/useNow';
import { syncLabel, useSync } from '../stores/sync';
import { detectHost, installEdgeResize, isMaximised } from '../utils/frame';
import ContextMenu, { type MenuItem } from './ContextMenu.vue';
import Icon from './Icon.vue';

// 无边框窗口的标题栏（spec D4）：应用菜单按钮（图标 +「LiteRSS」）、同步状态、拖动区，Windows 上还有三个窗口按钮。
// 应用菜单（spec D15）：立即同步（右侧附同步状态）、设置…。浏览器取证通道里没有宿主，窗口相关的部分都不生效。
const emit = defineEmits<{ settings: [] }>();
const host = detectHost();
const sync = useSync();
const now = useNow();
const syncText = computed(() => syncLabel(sync.state, now.value));
const menu = ref<{ x: number; y: number } | null>(null);

function toggleMenu(e: MouseEvent) {
  if (menu.value) {
    menu.value = null;
    return;
  }
  const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
  menu.value = { x: r.left, y: r.bottom + 4 };
}

const menuItems = computed<MenuItem[]>(() => [
  {
    label: '立即同步',
    icon: 'refresh',
    hint: syncText.value,
    disabled: !!sync.state?.running,
    run: () => void sync.runNow(),
  },
  { label: '设置…', icon: 'gear', sep: true, run: () => emit('settings') },
]);
// public/ 里的文件；写成绑定，模板编译不会把它当模块导入。按缩放比例取对齐像素格的那一版，
// 16px 显示的大图在 150% 下会糊。
const logo = '/assets/logo-16.svg';
const logoSrcset = [
  '/assets/logo-16.svg 1x',
  '/assets/logo-20.svg 1.25x',
  '/assets/logo-24.svg 1.5x',
  '/assets/logo-32.svg 2x',
].join(', ');
const maximised = ref(false);
let armed = false;
let removeResize = () => {};

const syncMaximised = () => {
  maximised.value = isMaximised();
};

onMounted(() => {
  if (host?.platform !== 'windows') return;
  syncMaximised();
  window.addEventListener('resize', syncMaximised);
  removeResize = installEdgeResize(host);
});
onBeforeUnmount(() => {
  window.removeEventListener('resize', syncMaximised);
  removeResize();
});

const onButton = (e: MouseEvent) => (e.target as Element).closest('button') !== null;

// 按下只做准备，移动了才开始拖：直接在按下时交给系统，双击就收不到了。
function onDown(e: MouseEvent) {
  armed = !!host && e.button === 0 && !onButton(e);
}

function onMove(e: MouseEvent) {
  if (!armed || !(e.buttons & 1)) return;
  armed = false;
  host!.post('wails:drag');
}

function onDblClick(e: MouseEvent) {
  if (!host || onButton(e)) return;
  // Wails 的双击消息只在 macOS 生效（跟随系统偏好）；Windows 走最大化按钮的接口。
  if (host.platform === 'mac') host.post('wails:drag:doubleclick');
  else void api.window.toggleMaximise();
}
</script>

<template>
  <header
    class="titlebar"
    :class="{ mac: host?.platform === 'mac' }"
    @mousedown="onDown"
    @mousemove="onMove"
    @mouseup="armed = false"
    @dblclick="onDblClick"
  >
    <button
      class="app-btn"
      :class="{ open: menu }"
      aria-haspopup="menu"
      :aria-expanded="!!menu"
      @click="toggleMenu"
    >
      <img class="logo" :src="logo" :srcset="logoSrcset" alt="" draggable="false" />
      <span class="name">LiteRSS</span>
      <Icon name="chev" class="caret" />
    </button>
    <span class="sync-state" :class="{ busy: sync.state?.running }" role="status">
      <Icon v-if="sync.state?.running" name="refresh" class="spin" />{{ syncText }}
    </span>
    <div v-if="host?.platform === 'windows'" class="win-buttons">
      <button aria-label="最小化" title="最小化" @click="api.window.minimise()">
        <Icon name="winMin" class="glyph" />
      </button>
      <button
        :aria-label="maximised ? '还原' : '最大化'"
        :title="maximised ? '还原' : '最大化'"
        @click="api.window.toggleMaximise()"
      >
        <Icon :name="maximised ? 'winRestore' : 'winMax'" class="glyph" />
      </button>
      <button class="close" aria-label="关闭" title="关闭" @click="api.window.close()">
        <Icon name="winClose" class="glyph" />
      </button>
    </div>
  </header>
  <!-- 放在顶栏外：遮罩上的按下不能冒泡成顶栏的拖动 -->
  <ContextMenu v-if="menu" :x="menu.x" :y="menu.y" :items="menuItems" @close="menu = null" />
</template>

<style scoped>
.titlebar {
  height: 32px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding-left: 6px;
  background: var(--bg-2);
  border-bottom: 1px solid var(--border);
  user-select: none;
}

/* macOS 红绿灯压在左上角；按钮自带 6px 内边距，图标仍落在原来的位置 */
.titlebar.mac {
  padding-left: 74px;
}

.app-btn {
  height: 24px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 4px 0 6px;
  border-radius: 6px;
}

.app-btn:hover,
.app-btn.open {
  background: var(--bg-hover);
}

.app-btn:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}

.logo {
  width: 16px;
  height: 16px;
  pointer-events: none;
}

.name {
  font-size: 12px;
  color: var(--text-2);
}

.caret {
  width: 12px;
  height: 12px;
  color: var(--text-3);
}

.sync-state {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  font-size: 12px;
  color: var(--text-3);
}

.sync-state .spin {
  width: 12px;
  height: 12px;
  flex: none;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.win-buttons {
  margin-left: auto;
  display: flex;
  align-self: stretch;
}

.win-buttons button {
  width: 46px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text);
  border-radius: 0;
}

.win-buttons button:hover {
  background: var(--bg-hover);
}

/* Windows 标题栏关闭按钮的系统红，不随主题变 */
.win-buttons .close:hover {
  background: #c42b1c;
  color: #fff;
}

.win-buttons .glyph {
  width: 10px;
  height: 10px;
  stroke-width: 2.4;
  stroke-linecap: butt;
  stroke-linejoin: miter;
}
</style>
