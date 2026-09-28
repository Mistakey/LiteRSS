<script setup lang="ts">
import { useSnackbar } from '../stores/snackbar';

const snackbar = useSnackbar();
</script>

<template>
  <div
    v-if="snackbar.current"
    class="snackbar"
    role="status"
    @mouseenter="snackbar.hold()"
    @mouseleave="snackbar.release()"
  >
    <span>{{ snackbar.current.text }}</span>
    <button v-if="snackbar.current.token" :disabled="snackbar.undoing" @click="snackbar.undo()">
      撤销
    </button>
  </div>
</template>

<style scoped>
.snackbar {
  position: fixed;
  left: 20px;
  bottom: 20px;
  z-index: 80;
  background: #23262d;
  color: #f0f2f5;
  padding: 9px 14px;
  border-radius: 8px;
  box-shadow: var(--shadow);
  display: flex;
  gap: 14px;
  align-items: center;
  font-size: 13px;
}

.snackbar button {
  color: #8fb3ff;
  font-weight: 600;
}
</style>
