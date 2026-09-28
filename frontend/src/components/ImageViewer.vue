<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { centered, draggable, fitScale, zoomAt, type Size, type View } from '../utils/viewer';
import Icon from './Icon.vue';

// 图片查看器（spec D15）：滚轮以指针为中心缩放，双击在适应窗口与原始大小之间切换，放大后拖动平移，
// 点背景 / 右上角 ✕ / Esc 关闭，多图左右切换。没有复制与下载。
const props = defineProps<{ images: string[]; start: number }>();
const emit = defineEmits<{ close: [] }>();

const index = ref(props.start);
const box = ref<HTMLElement>();
const natural = ref<Size | null>(null);
const failed = ref(false);
// 容器尺寸在挂载与窗口缩放时更新，canDrag 才跟得上。
const boxNow = ref<Size>({ width: 0, height: 0 });
const view = ref<View>({ scale: 1, x: 0, y: 0 });
const fit = ref(1);
const many = computed(() => props.images.length > 1);
const canDrag = computed(
  () => !!natural.value && draggable(natural.value, boxNow.value, view.value)
);
const imgStyle = computed(() => ({
  transform: `translate(${view.value.x}px, ${view.value.y}px) scale(${view.value.scale})`,
}));

function boxSize(): Size {
  const r = box.value?.getBoundingClientRect();
  boxNow.value = { width: r?.width ?? 0, height: r?.height ?? 0 };
  return boxNow.value;
}

function toFit() {
  if (!natural.value) return;
  fit.value = fitScale(natural.value, boxSize());
  view.value = centered(natural.value, boxSize(), fit.value);
}

function onLoad(e: Event) {
  const img = e.target as HTMLImageElement;
  failed.value = false;
  natural.value = { width: img.naturalWidth || 1, height: img.naturalHeight || 1 };
  toFit();
}

function go(step: number) {
  if (!many.value) return;
  index.value = (index.value + step + props.images.length) % props.images.length;
  natural.value = null;
  failed.value = false;
}

function point(e: MouseEvent) {
  const r = box.value!.getBoundingClientRect();
  return { px: e.clientX - r.left, py: e.clientY - r.top };
}

function onWheel(e: WheelEvent) {
  if (!natural.value) return;
  const { px, py } = point(e);
  const factor = e.deltaY < 0 ? 1.15 : 1 / 1.15;
  view.value = zoomAt(view.value, view.value.scale * factor, px, py, Math.min(fit.value, 1) / 2);
}

function onDblClick(e: MouseEvent) {
  if (!natural.value) return;
  if (Math.abs(view.value.scale - fit.value) < 0.001 && fit.value < 1) {
    const { px, py } = point(e);
    view.value = zoomAt(view.value, 1, px, py, fit.value);
  } else {
    toFit();
  }
}

let drag: { id: number; sx: number; sy: number; x: number; y: number } | null = null;
// 拖动结束时的 click 不当作点背景。
let dragged = false;

function onPointerDown(e: PointerEvent) {
  dragged = false;
  if (e.button !== 0 || !canDrag.value) return;
  e.preventDefault();
  (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
  drag = { id: e.pointerId, sx: e.clientX, sy: e.clientY, x: view.value.x, y: view.value.y };
}

function onPointerMove(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.id) return;
  const dx = e.clientX - drag.sx;
  const dy = e.clientY - drag.sy;
  if (Math.abs(dx) + Math.abs(dy) > 3) dragged = true;
  view.value = { ...view.value, x: drag.x + dx, y: drag.y + dy };
}

function onPointerUp(e: PointerEvent) {
  if (drag && e.pointerId === drag.id) drag = null;
}

function onBackground(e: MouseEvent) {
  if (dragged) {
    dragged = false;
    return;
  }
  if (!(e.target as Element).closest('img, button')) emit('close');
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close');
  else if (e.key === 'ArrowLeft') go(-1);
  else if (e.key === 'ArrowRight') go(1);
  else return;
  e.preventDefault();
  e.stopPropagation();
}

function onResize() {
  boxSize();
  if (natural.value && Math.abs(view.value.scale - fit.value) < 0.001) toFit();
}

onMounted(() => {
  boxSize();
  window.addEventListener('keydown', onKey, true);
  window.addEventListener('resize', onResize);
});
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey, true);
  window.removeEventListener('resize', onResize);
});
</script>

<template>
  <Teleport to="body">
    <div
      ref="box"
      class="viewer"
      role="dialog"
      aria-label="图片查看器"
      aria-modal="true"
      @click="onBackground"
      @wheel.prevent="onWheel"
    >
      <img
        :key="`${index}:${images[index]}`"
        :src="images[index]"
        :class="{ ready: natural, grab: canDrag }"
        :style="imgStyle"
        alt=""
        draggable="false"
        @load="onLoad"
        @error="failed = true"
        @dblclick="onDblClick"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
      />
      <div class="top">
        <span v-if="many" class="count">{{ index + 1 }} / {{ images.length }}</span>
        <span class="grow"></span>
        <button class="v-btn" title="关闭（Esc）" @click="emit('close')"><Icon name="x" /></button>
      </div>
      <template v-if="many">
        <button class="v-btn nav prev" title="上一张" @click="go(-1)"><Icon name="left" /></button>
        <button class="v-btn nav next" title="下一张" @click="go(1)"><Icon name="right" /></button>
      </template>
      <div v-if="failed" class="failed">图片载入失败</div>
      <div v-if="natural" class="zoom">{{ Math.round(view.scale * 100) }}%</div>
    </div>
  </Teleport>
</template>

<style scoped>
.viewer {
  position: fixed;
  inset: 0;
  z-index: 70;
  background: rgba(8, 10, 14, 0.92);
  overflow: hidden;
  user-select: none;
}

img {
  position: absolute;
  left: 0;
  top: 0;
  max-width: none;
  transform-origin: 0 0;
  visibility: hidden;
  border-radius: 2px;
}

img.ready {
  visibility: visible;
}

img.grab {
  cursor: grab;
}

img.grab:active {
  cursor: grabbing;
}

.top {
  position: absolute;
  top: 14px;
  left: 18px;
  right: 14px;
  display: flex;
  align-items: center;
  color: #ccd;
  font-size: 13px;
}

.grow {
  flex: 1;
}

.v-btn {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  color: #eef;
  background: rgba(255, 255, 255, 0.08);
}

.v-btn:hover {
  background: rgba(255, 255, 255, 0.18);
}

.nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 46px;
  height: 46px;
}

.prev {
  left: 20px;
}

.next {
  right: 20px;
}

.failed {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: #ccd;
  font-size: 14px;
}

.zoom {
  position: absolute;
  bottom: 20px;
  left: 50%;
  transform: translateX(-50%);
  padding: 3px 10px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.08);
  color: #ccd;
  font-size: 12px;
}
</style>
