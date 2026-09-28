<script setup lang="ts">
import { computed, nextTick, ref } from 'vue';
import { useSettings, type SecretKey } from '../stores/settings';

// 设置面板里的一个凭据（spec D10）。凭据从不回显：已保存的是不可编辑的掩码，配「修改」「清除」；
// 修改中可以取消，清除在保存前可以撤销；没保存过的是普通输入框。外面的 <label for> 指向 name。
const props = withDefaults(
  defineProps<{ name: SecretKey; password?: boolean; placeholder?: string }>(),
  { password: true, placeholder: '' }
);

const MASK = '••••••••';

const settings = useSettings();
const state = computed(() => settings.secretState(props.name));
const input = ref<HTMLInputElement>();

async function edit() {
  settings.editSecret(props.name);
  await nextTick();
  input.value?.focus();
}
</script>

<template>
  <div class="secret" :data-secret="name">
    <template v-if="state === 'saved'">
      <input :id="name" class="masked" :value="MASK" readonly aria-description="已保存" />
      <button type="button" class="link" @click="edit">修改</button>
      <button type="button" class="link" @click="settings.clearSecret(name)">清除</button>
    </template>
    <template v-else-if="state === 'cleared'">
      <span :id="name" class="cleared">保存后清除</span>
      <button type="button" class="link" @click="settings.undoClear(name)">撤销</button>
    </template>
    <template v-else>
      <input
        :id="name"
        ref="input"
        v-model="settings.draft[name]"
        :name="name"
        :type="password ? 'password' : 'text'"
        autocomplete="off"
        spellcheck="false"
        :placeholder="state === 'editing' ? '填写新的值' : placeholder"
      />
      <button
        v-if="state === 'editing'"
        type="button"
        class="link"
        @click="settings.cancelEdit(name)"
      >
        取消
      </button>
    </template>
  </div>
</template>

<style scoped>
.secret {
  display: flex;
  align-items: center;
  gap: 8px;
}

input {
  width: 100%;
  min-width: 0;
  height: 32px;
  padding: 0 10px;
  font: inherit;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
}

input::placeholder {
  color: var(--text-3);
}

input.masked {
  color: var(--text-2);
  background: var(--bg-2);
  cursor: default;
}

.cleared {
  flex: 1;
  min-width: 0;
  height: 32px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  color: var(--danger);
  border: 1px dashed var(--border);
  border-radius: 6px;
}

.link {
  flex: none;
  color: var(--accent);
  font-size: 13px;
}

input:focus-visible,
.link:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 1px;
}
</style>
