/**
 * 设置面板的表单（spec D10）：载入一份设置作为草稿，保存时只提交清单内改过的键。
 * 凭据从不回显：已保存的显示为掩码，「修改」后填的值才提交，「清除」保存时经 clearSecret 删除（可撤销）；
 * 测试连接同样只传填了或清除了的凭据。
 */
import { defineStore } from 'pinia';
import { computed, reactive, ref, shallowRef } from 'vue';
import { api, ApiError, type ConnectionTest, type SettingsView } from '../api';
import { settingsDefaults, type SettingsData } from '../types/settings.generated';

type Key = keyof SettingsData;

/** 面板能写的键；生成的默认值只含 schema 里非内部的键。 */
export const SETTING_KEYS = Object.keys(settingsDefaults) as Key[];

export type SecretKey =
  | 'freshrss_api_password'
  | 'llm_api_key'
  | 'baidu_secret_key'
  | 'proxy_username'
  | 'proxy_password';

export const SECRET_KEYS: readonly SecretKey[] = [
  'freshrss_api_password',
  'llm_api_key',
  'baidu_secret_key',
  'proxy_username',
  'proxy_password',
];

const FRESHRSS_FORM: readonly Key[] = [
  'freshrss_server_url',
  'freshrss_username',
  'freshrss_api_password',
];
const MODEL_FORM: readonly Key[] = ['llm_endpoint', 'llm_model', 'llm_api_key'];

export type TestName = 'freshrss' | 'model';

/** saved：已保存，显示掩码；editing：已保存、正在填新值；cleared：保存时清除；empty：没保存过。 */
export type SecretState = 'saved' | 'editing' | 'cleared' | 'empty';

/** 测试进行中为 running，结束后是后端的结果。 */
export type TestState = { running: true } | ({ running: false } & ConnectionTest);

function isSecret(key: Key): key is SecretKey {
  return (SECRET_KEYS as readonly Key[]).includes(key);
}

function errorText(err: unknown) {
  if (err instanceof ApiError && err.status === 400 && err.message) {
    return `设置没有保存：${err.message}`;
  }
  if (err instanceof ApiError && err.status === 0) return '连不上 LiteRSS 后端，请稍后再试。';
  return '设置没有保存，请稍后再试。';
}

export const useSettings = defineStore('settings', () => {
  const loaded = shallowRef<SettingsView | null>(null);
  const draft = reactive<SettingsData>({ ...settingsDefaults });
  /** 用户点了「清除」的已存凭据。 */
  const cleared = reactive(new Set<Key>());
  /** 用户点了「修改」的已存凭据。 */
  const editing = reactive(new Set<Key>());
  const loadError = ref('');
  const saving = ref(false);
  const saveError = ref('');
  const tests = reactive<Partial<Record<TestName, TestState>>>({});

  function reset(view: SettingsView) {
    loaded.value = view;
    Object.assign(draft, view.settings);
    cleared.clear();
    editing.clear();
  }

  async function load() {
    loadError.value = '';
    saveError.value = '';
    delete tests.freshrss;
    delete tests.model;
    try {
      reset(await api.settings());
    } catch {
      loaded.value = null;
      loadError.value = '读取设置失败，请关闭后重试。';
    }
  }

  function secretState(key: Key): SecretState {
    if (cleared.has(key)) return 'cleared';
    if (!loaded.value?.saved_secrets.includes(key)) return 'empty';
    return editing.has(key) ? 'editing' : 'saved';
  }

  function setDraft(key: Key, value: string) {
    (draft as Record<Key, unknown>)[key] = value;
  }

  /** 修改已保存的凭据：给一个空输入框，取消即恢复。 */
  function editSecret(key: Key) {
    editing.add(key);
    setDraft(key, '');
  }

  function cancelEdit(key: Key) {
    editing.delete(key);
    setDraft(key, '');
  }

  /** 清除已保存的凭据：保存时才删，之前可以撤销。 */
  function clearSecret(key: Key) {
    editing.delete(key);
    cleared.add(key);
    setDraft(key, '');
  }

  function undoClear(key: Key) {
    cleared.delete(key);
  }

  /** 一个键的待提交值；undefined 表示不改。凭据只提交填了的值，清除另走 clearSecret。 */
  function pending(key: Key): SettingsData[Key] | undefined {
    const value = draft[key];
    if (isSecret(key)) return value !== '' && !cleared.has(key) ? value : undefined;
    return value === loaded.value?.settings[key] ? undefined : value;
  }

  const changes = computed(() => {
    const out: Partial<Record<Key, SettingsData[Key]>> = {};
    for (const key of SETTING_KEYS) {
      const v = pending(key);
      if (v !== undefined) out[key] = v;
    }
    return out as Partial<SettingsData>;
  });

  const intervalValid = computed(
    () =>
      Number.isInteger(draft.freshrss_auto_sync_interval) && draft.freshrss_auto_sync_interval >= 1
  );

  const dirty = computed(() => Object.keys(changes.value).length > 0 || cleared.size > 0);

  /** 保存改过的键；成功返回 true，失败把原因放进 saveError。 */
  async function save() {
    if (!dirty.value) return true;
    if (!intervalValid.value) {
      saveError.value = '同步间隔要是不小于 1 的整数分钟。';
      return false;
    }
    saving.value = true;
    saveError.value = '';
    try {
      // 先写改动再清除：改动被拒收时什么也不删。两步都幂等，失败后再点保存会重做。
      let view: SettingsView | null = null;
      if (Object.keys(changes.value).length) view = await api.updateSettings(changes.value);
      for (const key of [...cleared]) view = await api.clearSecret(key);
      if (view) reset(view);
      return true;
    } catch (err) {
      saveError.value = errorText(err);
      return false;
    } finally {
      saving.value = false;
    }
  }

  /** 测试连接用表单里当前的值；没填也没清除的凭据不传，后端取已存值。 */
  async function test(name: TestName) {
    const form: Partial<Record<Key, SettingsData[Key]>> = {};
    for (const key of name === 'freshrss' ? FRESHRSS_FORM : MODEL_FORM) {
      if (isSecret(key)) {
        const v = cleared.has(key) ? '' : pending(key);
        if (v !== undefined) form[key] = v;
      } else {
        form[key] = draft[key];
      }
    }
    tests[name] = { running: true };
    let result: ConnectionTest;
    try {
      const call = name === 'freshrss' ? api.testFreshRSS : api.testModel;
      result = await call(form as Partial<SettingsData>);
    } catch {
      result = { ok: false, message: '测试没有完成，请稍后再试。' };
    }
    tests[name] = { running: false, ...result };
  }

  return {
    loaded,
    draft,
    loadError,
    saving,
    saveError,
    tests,
    changes,
    dirty,
    intervalValid,
    load,
    secretState,
    editSecret,
    cancelEdit,
    clearSecret,
    undoClear,
    save,
    test,
  };
});
