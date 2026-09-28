<script setup lang="ts">
import { computed } from 'vue';
import { badgeColor, badgeText } from '../utils/format';

// 源的字母徽标：不加载源图标，避免对外请求。
const props = defineProps<{ id: string; title: string }>();
const color = computed(() => badgeColor(props.id));
const text = computed(() => badgeText(props.title));
</script>

<template>
  <span class="fav" :class="color" aria-hidden="true">{{ text }}</span>
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
