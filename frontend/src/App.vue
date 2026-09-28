<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import AppSidebar from './components/AppSidebar.vue';
import ArticleDetail from './components/ArticleDetail.vue';
import ArticleList from './components/ArticleList.vue';
import SettingsModal from './components/SettingsModal.vue';
import TitleBar from './components/TitleBar.vue';
import UndoSnackbar from './components/UndoSnackbar.vue';
import { useReader } from './stores/reader';
import { useSync } from './stores/sync';

// 顶栏兼作无边框窗口的标题栏（spec D4）；其下三栏按布局原型变体 B（spec D15）：侧栏、列表、详情。
const reader = useReader();
const sync = useSync();
const settingsOpen = ref(false);

onMounted(() => {
  void reader.init();
  sync.start(() => reader.afterSync());
});
onBeforeUnmount(() => sync.stop());
</script>

<template>
  <div class="app">
    <TitleBar />
    <div class="panes">
      <AppSidebar @settings="settingsOpen = true" />
      <ArticleList />
      <ArticleDetail />
    </div>
    <UndoSnackbar />
    <SettingsModal v-if="settingsOpen" @close="settingsOpen = false" />
  </div>
</template>

<style scoped>
.app {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.panes {
  flex: 1;
  display: flex;
  min-width: 0;
  min-height: 0;
}
</style>
