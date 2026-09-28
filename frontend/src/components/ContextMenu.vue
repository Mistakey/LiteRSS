<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue';
import Icon from './Icon.vue';
import type { IconName } from './icons';

export interface MenuItem {
  label: string;
  icon: IconName;
  hint?: string;
  disabled?: boolean;
  /** 在这一项之前画分隔线。 */
  sep?: boolean;
  run: () => void;
}

const props = defineProps<{ x: number; y: number; items: MenuItem[]; caption?: string }>();
const emit = defineEmits<{ close: [] }>();

// 靠近窗口右下边缘时往里收，菜单不出界。
const pos = computed(() => ({
  left: `${Math.min(props.x, window.innerWidth - 250)}px`,
  top: `${Math.min(props.y, window.innerHeight - (props.items.length * 36 + 40))}px`,
}));

function pick(item: MenuItem) {
  if (item.disabled) return;
  emit('close');
  item.run();
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close');
}
function close() {
  emit('close');
}

onMounted(() => {
  document.addEventListener('keydown', onKey);
  window.addEventListener('resize', close);
  window.addEventListener('blur', close);
  document.addEventListener('scroll', close, true);
});
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey);
  window.removeEventListener('resize', close);
  window.removeEventListener('blur', close);
  document.removeEventListener('scroll', close, true);
});
</script>

<template>
  <div class="mask" @mousedown="close" @contextmenu.prevent="close">
    <div class="ctx" role="menu" :style="pos" @mousedown.stop>
      <div v-if="caption" class="cap">{{ caption }}</div>
      <template v-for="item in items" :key="item.label">
        <div v-if="item.sep" class="sep"></div>
        <button role="menuitem" :disabled="item.disabled" @click="pick(item)">
          <Icon :name="item.icon" />{{ item.label }}
          <span v-if="item.hint" class="hint">{{ item.hint }}</span>
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.mask {
  position: fixed;
  inset: 0;
  z-index: 60;
}

.ctx {
  position: fixed;
  min-width: 210px;
  padding: 5px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 9px;
  box-shadow: var(--shadow);
}

.ctx button {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 7px 10px;
  border-radius: 6px;
  text-align: left;
}

.ctx button:hover:not(:disabled) {
  background: var(--bg-hover);
}

.ctx button:disabled {
  opacity: 0.5;
  cursor: default;
}

.sep {
  height: 1px;
  background: var(--border);
  margin: 4px 6px;
}

.hint {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-3);
}

.cap {
  padding: 4px 10px 2px;
  font-size: 11.5px;
  color: var(--text-3);
}
</style>
