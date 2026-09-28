<script setup lang="ts">
import { computed } from 'vue';
import { icons, type IconName } from './icons';

const props = defineProps<{ name: IconName }>();
const shapes = computed(() => icons[props.name]);
</script>

<template>
  <svg class="icon" viewBox="0 0 24 24" aria-hidden="true">
    <component :is="tag" v-for="([tag, attrs], i) in shapes" :key="i" v-bind="attrs" />
  </svg>
</template>

<style scoped>
.icon {
  width: 18px;
  height: 18px;
  flex: none;
  stroke: currentColor;
  fill: none;
  stroke-width: 1.7;
  stroke-linecap: round;
  stroke-linejoin: round;
}
</style>
