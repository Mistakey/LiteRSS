<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { api, type UpdateCheck, type UpdateStatus } from '../api';
import { useSettings, type TestName } from '../stores/settings';
import type { SettingsData } from '../types/settings.generated';
import Icon from './Icon.vue';
import SecretField from './SecretField.vue';

// 设置面板（spec D10）：一个模态框、一页滚动，分组依次为 FreshRSS、摘要模型、标题翻译、网络代理、应用、关于。
const emit = defineEmits<{ close: [] }>();
const settings = useSettings();
const d = settings.draft;

const version = ref('');
const update = ref<UpdateCheck | null>(null);
const checking = ref(false);

function testLabel(name: TestName) {
  return settings.tests[name]?.running ? '正在测试…' : '测试连接';
}

async function save() {
  if (await settings.save()) emit('close');
}

async function checkUpdate() {
  checking.value = true;
  progress.value = null;
  try {
    update.value = await api.checkUpdate();
  } catch {
    update.value = {
      current_version: version.value,
      latest_version: '',
      update_available: false,
      release_url: '',
      in_app: false,
      message: '检查更新没有完成，请稍后再试。',
    };
  } finally {
    checking.value = false;
  }
}

function updateText(u: UpdateCheck) {
  if (u.message) return u.message;
  if (u.update_available) return `有新版本 ${u.latest_version}。`;
  return '已是最新版本。';
}

// 应用内更新（spec D20）：点「更新到 X」后端一路下载、校验、安装并退出；面板只轮询进度。
// 托盘对话框开始的更新在打开面板时同样显示。
const progress = ref<UpdateStatus | null>(null);
let pollTimer: ReturnType<typeof setTimeout> | undefined;
const POLL_MS = 500;

function busy(s: UpdateStatus | null) {
  return !!s && ['downloading', 'verifying', 'installing'].includes(s.state);
}

function poll() {
  clearTimeout(pollTimer);
  if (!busy(progress.value)) return;
  pollTimer = setTimeout(async () => {
    try {
      progress.value = await api.updateStatus();
    } catch {
      // 应用退出去安装时请求会失败，保留最后的进度。
    }
    poll();
  }, POLL_MS);
}

async function startUpdate() {
  try {
    progress.value = await api.startUpdate();
  } catch {
    progress.value = {
      state: 'failed',
      version: update.value?.latest_version ?? '',
      received: 0,
      total: 0,
      message: '更新没有开始，请稍后再试。',
      release_url: update.value?.release_url ?? '',
    };
  }
  poll();
}

function mb(bytes: number) {
  return (bytes / 1048576).toFixed(1);
}

function progressText(s: UpdateStatus) {
  switch (s.state) {
    case 'downloading':
      return s.total
        ? `正在下载 ${s.version}：${Math.floor((s.received / s.total) * 100)}%（${mb(s.received)} / ${mb(s.total)} MB）`
        : `正在下载 ${s.version}：${mb(s.received)} MB`;
    case 'verifying':
      return '正在校验安装包…';
    case 'installing':
      return '正在安装，LiteRSS 会退出并重新启动…';
    default:
      return s.message;
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close');
}

onMounted(() => {
  document.addEventListener('keydown', onKey);
  void settings.load();
  api.version().then(
    (v) => (version.value = v),
    () => (version.value = '')
  );
  api.updateStatus().then(
    (s) => {
      if (s.state !== 'idle') {
        progress.value = s;
        poll();
      }
    },
    () => undefined
  );
});
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey);
  clearTimeout(pollTimer);
});

const proxyModes: { value: SettingsData['proxy_mode']; label: string }[] = [
  { value: 'system', label: '跟随系统' },
  { value: 'direct', label: '不用代理' },
  { value: 'manual', label: '手动' },
];
</script>

<template>
  <div class="mask" @mousedown.self="emit('close')">
    <section class="modal" role="dialog" aria-modal="true" aria-labelledby="settings-title">
      <header class="head">
        <h2 id="settings-title">设置</h2>
        <span class="grow"></span>
        <button class="icon-btn" title="关闭" aria-label="关闭" @click="emit('close')">
          <Icon name="x" />
        </button>
      </header>

      <div v-if="settings.loadError" class="body">
        <p class="load-error" role="alert">{{ settings.loadError }}</p>
      </div>
      <div v-else-if="!settings.loaded" class="body">
        <p class="tip">正在读取设置…</p>
      </div>
      <form v-else id="settings-form" class="body scroll" novalidate @submit.prevent="save">
        <fieldset class="group" data-group="freshrss">
          <legend>FreshRSS</legend>
          <label class="row">
            <span class="lbl">服务器地址</span>
            <input
              v-model.trim="d.freshrss_server_url"
              name="freshrss_server_url"
              type="url"
              placeholder="https://freshrss.example.com"
              spellcheck="false"
            />
          </label>
          <label class="row">
            <span class="lbl">用户名</span>
            <input v-model.trim="d.freshrss_username" name="freshrss_username" spellcheck="false" />
          </label>
          <div class="row">
            <label class="lbl" for="freshrss_api_password">API 密码</label>
            <SecretField name="freshrss_api_password" />
          </div>
          <label class="row">
            <span class="lbl">同步间隔</span>
            <span class="with-unit">
              <input
                v-model.number="d.freshrss_auto_sync_interval"
                name="freshrss_auto_sync_interval"
                type="number"
                min="1"
                step="1"
                class="short"
                :aria-invalid="!settings.intervalValid"
              />
              分钟
            </span>
          </label>
          <p class="tip">API 密码是 FreshRSS「个人资料 → API 管理」里设的密码，不是登录密码。</p>
          <div class="test">
            <button
              type="button"
              class="btn"
              :disabled="settings.tests.freshrss?.running"
              @click="settings.test('freshrss')"
            >
              {{ testLabel('freshrss') }}
            </button>
            <span
              v-if="settings.tests.freshrss && !settings.tests.freshrss.running"
              class="result"
              :class="settings.tests.freshrss.ok ? 'ok' : 'fail'"
              role="status"
              data-test="freshrss"
            >
              {{ settings.tests.freshrss.message }}
            </span>
          </div>
        </fieldset>

        <fieldset class="group" data-group="llm">
          <legend>摘要模型</legend>
          <label class="row">
            <span class="lbl">接口地址</span>
            <input
              v-model.trim="d.llm_endpoint"
              name="llm_endpoint"
              type="url"
              placeholder="https://api.openai.com/v1/chat/completions"
              spellcheck="false"
            />
          </label>
          <label class="row">
            <span class="lbl">模型</span>
            <input v-model.trim="d.llm_model" name="llm_model" spellcheck="false" />
          </label>
          <div class="row">
            <label class="lbl" for="llm_api_key">API 密钥</label>
            <SecretField name="llm_api_key" placeholder="本地模型可留空" />
          </div>
          <p class="tip">OpenAI 兼容接口。只用来给英文文章生成中文摘要，点「摘要」时才调用。</p>
          <div class="test">
            <button
              type="button"
              class="btn"
              :disabled="settings.tests.model?.running"
              @click="settings.test('model')"
            >
              {{ testLabel('model') }}
            </button>
            <span
              v-if="settings.tests.model && !settings.tests.model.running"
              class="result"
              :class="settings.tests.model.ok ? 'ok' : 'fail'"
              role="status"
              data-test="model"
            >
              {{ settings.tests.model.message }}
            </span>
          </div>
        </fieldset>

        <fieldset class="group" data-group="translation">
          <legend>标题翻译</legend>
          <label class="row">
            <span class="lbl">百度 APP ID</span>
            <input v-model.trim="d.baidu_app_id" name="baidu_app_id" spellcheck="false" />
          </label>
          <div class="row">
            <label class="lbl" for="baidu_secret_key">百度密钥</label>
            <SecretField name="baidu_secret_key" />
          </div>
          <p class="tip">英文标题经百度通用翻译译成中文，列表里只显示译文。</p>
        </fieldset>

        <fieldset class="group" data-group="proxy">
          <legend>网络代理</legend>
          <div class="row">
            <span class="lbl">代理</span>
            <div class="seg" role="radiogroup" aria-label="代理">
              <label
                v-for="m in proxyModes"
                :key="m.value"
                :class="{ on: d.proxy_mode === m.value }"
              >
                <input v-model="d.proxy_mode" type="radio" name="proxy_mode" :value="m.value" />
                {{ m.label }}
              </label>
            </div>
          </div>
          <template v-if="d.proxy_mode === 'manual'">
            <label class="row">
              <span class="lbl">类型</span>
              <select v-model="d.proxy_type" name="proxy_type" class="short">
                <option value="http">HTTP</option>
                <option value="https">HTTPS</option>
                <option value="socks5">SOCKS5</option>
              </select>
            </label>
            <div class="row">
              <span class="lbl">地址</span>
              <div class="host">
                <input
                  v-model.trim="d.proxy_host"
                  name="proxy_host"
                  aria-label="主机"
                  spellcheck="false"
                />
                <span>:</span>
                <input
                  v-model.trim="d.proxy_port"
                  name="proxy_port"
                  aria-label="端口"
                  inputmode="numeric"
                  class="port"
                />
              </div>
            </div>
            <div class="row">
              <label class="lbl" for="proxy_username">用户名</label>
              <SecretField name="proxy_username" :password="false" placeholder="不需要认证可留空" />
            </div>
            <div class="row">
              <label class="lbl" for="proxy_password">密码</label>
              <SecretField name="proxy_password" />
            </div>
          </template>
          <p class="tip">
            用于抓全文、标题翻译、摘要与检查更新；连 FreshRSS 不走代理。保存后立即生效。
          </p>
        </fieldset>

        <fieldset class="group" data-group="app">
          <legend>应用</legend>
          <label class="check">
            <input v-model="d.close_to_tray" type="checkbox" name="close_to_tray" />
            关闭窗口时留在托盘
          </label>
          <label class="check">
            <input v-model="d.startup_on_boot" type="checkbox" name="startup_on_boot" />
            开机时启动
          </label>
          <label class="check">
            <input v-model="d.update_check_enabled" type="checkbox" name="update_check_enabled" />
            有新版本时提示
          </label>
        </fieldset>

        <fieldset class="group" data-group="about">
          <legend>关于</legend>
          <p class="about">
            LiteRSS <span v-if="version" class="ver">{{ version }}</span>
            <span class="tip">FreshRSS 未读阅读器</span>
          </p>
          <div class="test">
            <button
              type="button"
              class="btn"
              :disabled="checking || busy(progress)"
              @click="checkUpdate"
            >
              {{ checking ? '正在检查…' : '检查更新' }}
            </button>
            <template v-if="update && !progress">
              <span class="result" role="status" data-test="update">
                {{ updateText(update) }}
              </span>
              <button
                v-if="update.update_available && update.in_app"
                type="button"
                class="btn primary"
                data-test="start-update"
                @click="startUpdate"
              >
                更新到 {{ update.latest_version }}
              </button>
              <button
                v-else-if="update.update_available && update.release_url"
                type="button"
                class="link"
                @click="api.openInBrowser(update.release_url)"
              >
                去发布页
              </button>
            </template>
          </div>
          <div v-if="progress" class="test" data-test="update-progress">
            <progress
              v-if="progress.state === 'downloading' && progress.total"
              class="bar"
              :value="progress.received"
              :max="progress.total"
            />
            <span
              class="result"
              :class="{ fail: progress.state === 'failed' }"
              :role="progress.state === 'failed' ? 'alert' : 'status'"
            >
              {{ progressText(progress) }}
            </span>
            <button
              v-if="progress.state === 'failed' && progress.release_url"
              type="button"
              class="link"
              @click="api.openInBrowser(progress.release_url)"
            >
              去发布页
            </button>
          </div>
        </fieldset>
      </form>

      <footer class="foot">
        <span v-if="settings.saveError" class="save-error" role="alert">{{
          settings.saveError
        }}</span>
        <span class="grow"></span>
        <button type="button" class="btn" @click="emit('close')">取消</button>
        <button
          type="submit"
          form="settings-form"
          class="btn primary"
          :disabled="!settings.loaded || !settings.dirty || settings.saving"
        >
          {{ settings.saving ? '正在保存…' : '保存' }}
        </button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.mask {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: grid;
  place-items: center;
  background: rgba(15, 23, 42, 0.32);
}

.modal {
  width: min(620px, calc(100vw - 32px));
  max-height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: var(--shadow);
}

.head {
  flex: none;
  display: flex;
  align-items: center;
  padding: 12px 12px 12px 20px;
  border-bottom: 1px solid var(--border);
}

.head h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.body {
  padding: 4px 20px 16px;
}

.group {
  margin: 0;
  padding: 14px 0 16px;
  border: 0;
  border-bottom: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.group:last-child {
  border-bottom: 0;
}

legend {
  float: left;
  width: 100%;
  padding: 0;
  margin-bottom: 4px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-2);
}

.row {
  display: grid;
  grid-template-columns: 96px 1fr;
  align-items: center;
  gap: 12px;
}

.lbl {
  color: var(--text-2);
}

input:not([type='checkbox'], [type='radio']),
select {
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

input:focus-visible,
select:focus-visible,
.btn:focus-visible,
.link:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 1px;
}

input[aria-invalid='true'] {
  border-color: var(--danger);
}

input::placeholder {
  color: var(--text-3);
}

.short {
  max-width: 120px;
}

.with-unit {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-2);
}

.host {
  display: flex;
  align-items: center;
  gap: 8px;
}

.host .port {
  width: 88px;
  flex: none;
}

.seg {
  display: inline-flex;
  justify-self: start;
  padding: 3px;
  background: var(--bg-3);
  border-radius: 8px;
}

.seg label {
  position: relative;
  padding: 4px 12px;
  border-radius: 6px;
  color: var(--text-2);
  cursor: pointer;
}

.seg label.on {
  background: var(--bg);
  color: var(--text);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
}

.seg input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

.seg label:has(input:focus-visible) {
  outline: 2px solid var(--accent);
}

.check {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.check input {
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--accent);
}

.tip {
  margin: 0;
  font-size: 12px;
  color: var(--text-3);
}

.test {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.result {
  font-size: 13px;
}

.result.ok {
  color: var(--accent);
}

.result.fail,
.save-error,
.load-error {
  color: var(--danger);
}

.btn {
  height: 32px;
  padding: 0 14px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg);
}

.btn:hover:not(:disabled) {
  background: var(--bg-hover);
}

.btn:disabled {
  opacity: 0.55;
  cursor: default;
}

.btn.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
}

.btn.primary:hover:not(:disabled) {
  background: var(--accent);
  filter: brightness(1.08);
}

.link {
  flex: none;
  color: var(--accent);
  font-size: 13px;
}

.bar {
  width: 160px;
  height: 6px;
  accent-color: var(--accent);
}

.about {
  margin: 0;
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-weight: 600;
}

.about .ver {
  font-weight: 400;
  color: var(--text-2);
  font-variant-numeric: tabular-nums;
}

.about .tip {
  font-weight: 400;
}

.foot {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 20px;
  border-top: 1px solid var(--border);
}

.save-error {
  font-size: 13px;
}
</style>
