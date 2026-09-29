<script lang="ts">
// 取不到的图标地址（后端回不缓存的 204），各实例共用：同一个源在列表里出现多次时只失败一次，不反复请求。
const failed = new Set<string>();
</script>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useReader } from '../stores/reader';
import { badgeColor, badgeText } from '../utils/format';

// 源徽标：有图标用后端从 FreshRSS 取来的图标（spec D15，界面不直连外部），没有或取不到时用按 ID 取色的字母徽标。
const props = defineProps<{ id: string; title: string }>();
const reader = useReader();
const color = computed(() => badgeColor(props.id));
const text = computed(() => badgeText(props.title));
// 让 failed 的变化触发重算。
const failures = ref(0);
const src = computed(() => {
  void failures.value;
  const url = reader.iconFor(props.id);
  return url && !failed.has(url) ? url : '';
});

function onError() {
  failed.add(src.value);
  failures.value++;
}
</script>

<template>
  <img v-if="src" class="fav icon" :src="src" alt="" aria-hidden="true" @error="onError" />
  <span v-else class="fav" :class="color" aria-hidden="true">{{ text }}</span>
</template>

<style scoped>
.fav {
  width: 16px;
  height: 16px;
  border-radius: 4px;
  color: #fff;
  font-size: 10px;
  line-height: 16px;
  text-align: center;
  font-weight: 700;
  flex: none;
}
.icon {
  object-fit: contain;
}
.c0 {
  background: #e0633a;
}
.c1 {
  background: #2f6fed;
}
.c2 {
  background: #15a37b;
}
.c3 {
  background: #9b59b6;
}
.c4 {
  background: #d4a017;
}
.c5 {
  background: #e04868;
}
.c6 {
  background: #1f8fb3;
}
.c7 {
  background: #6b7280;
}
</style>
