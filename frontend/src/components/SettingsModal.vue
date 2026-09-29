<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { api, type UpdateCheck, type UpdateStatus } from '../api';
import { SETTING_GROUPS, useSettings, type GroupId, type TestName } from '../stores/settings';
import type { SettingsData } from '../types/settings.generated';
import Icon from './Icon.vue';
import SecretField from './SecretField.vue';

// 设置面板（spec D10）：固定大小的模态框，左侧分组导航，右侧一次只显示一组，底部取消 / 保存覆盖所有分组的改动。
// 各组用 v-show 切换而不卸载：草稿、测试结果与检查更新的状态在切换分组后都还在。
const emit = defineEmits<{ close: [] }>();
const settings = useSettings();
const d = settings.draft;

const group = ref<GroupId>('freshrss');
const groupLabel = computed(() => SETTING_GROUPS.find((g) => g.id === group.value)!.label);
const intervalInput = ref<HTMLInputElement>();

const version = ref('');
const update = ref<UpdateCheck | null>(null);
const checking = ref(false);

function testLabel(name: TestName) {
  return settings.tests[name]?.running ? '正在测试…' : '测试连接';
}

async function save() {
  if (await settings.save()) {
    emit('close');
    return;
  }
  // 保存被拒在不合法的值上时跳到那一组，报错才对得上用户眼前的输入框。
  const bad = SETTING_GROUPS.find((g) => settings.invalidGroups.has(g.id));
  if (!bad) return;
  group.value = bad.id;
  if (bad.id === 'freshrss') {
    await nextTick();
    intervalInput.value?.focus();
  }
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
      <nav class="side" aria-label="设置分组">
        <h2 id="settings-title">设置</h2>
        <button
          v-for="g in SETTING_GROUPS"
          :key="g.id"
          type="button"
          class="nav-item"
          :class="{ on: group === g.id }"
          :aria-current="group === g.id ? 'true' : undefined"
          :data-nav="g.id"
          @click="group = g.id"
        >
          {{ g.label }}
          <span
            v-if="settings.invalidGroups.has(g.id)"
            class="mark invalid"
            title="有不合法的值"
            aria-label="有不合法的值"
          ></span>
          <span
            v-else-if="settings.dirtyGroups.has(g.id)"
            class="mark dirty"
            title="有未保存的改动"
            aria-label="有未保存的改动"
          ></span>
        </button>
      </nav>

      <div class="main">
        <header class="head">
          <h3>{{ groupLabel }}</h3>
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
          <fieldset
            v-show="group === 'freshrss'"
            class="group"
            data-group="freshrss"
            aria-label="FreshRSS"
          >
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
              <input
                v-model.trim="d.freshrss_username"
                name="freshrss_username"
                spellcheck="false"
              />
            </label>
            <div class="row">
              <label class="lbl" for="freshrss_api_password">API 密码</label>
              <SecretField name="freshrss_api_password" />
            </div>
            <label class="row">
              <span class="lbl">同步间隔</span>
              <span class="with-unit">
                <input
                  ref="intervalInput"
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

          <fieldset v-show="group === 'llm'" class="group" data-group="llm" aria-label="大模型">
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
            <p class="tip">
              OpenAI 兼容接口。用来生成中文摘要和全文翻译，点「摘要」或「翻译」时才调用。
            </p>
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

          <fieldset
            v-show="group === 'translation'"
            class="group"
            data-group="translation"
            aria-label="标题翻译"
          >
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

          <fieldset
            v-show="group === 'proxy'"
            class="group"
            data-group="proxy"
            aria-label="网络代理"
          >
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
                <SecretField
                  name="proxy_username"
                  :password="false"
                  placeholder="不需要认证可留空"
                />
              </div>
              <div class="row">
                <label class="lbl" for="proxy_password">密码</label>
                <SecretField name="proxy_password" />
              </div>
            </template>
            <p class="tip">
              用于抓全文、标题翻译、摘要、全文翻译与检查更新；连 FreshRSS 不走代理。保存后立即生效。
            </p>
          </fieldset>

          <fieldset v-show="group === 'app'" class="group" data-group="app" aria-label="应用">
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

          <fieldset v-show="group === 'about'" class="group" data-group="about" aria-label="关于">
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
      </div>
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
  width: min(780px, calc(100vw - 32px));
  height: min(520px, calc(100vh - 48px));
  display: flex;
  overflow: hidden;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: var(--shadow);
}

.side {
  width: 168px;
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 16px 10px;
  background: var(--bg-2);
  border-right: 1px solid var(--border);
}

.side h2 {
  margin: 0 8px 10px;
  font-size: 16px;
  font-weight: 600;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 10px;
  border-radius: 6px;
  text-align: left;
  color: var(--text-2);
}

.nav-item:hover {
  background: var(--bg-hover);
}

.nav-item.on {
  background: var(--bg-sel);
  color: var(--accent);
  font-weight: 500;
}

.nav-item:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}

.mark {
  width: 7px;
  height: 7px;
  margin-left: auto;
  border-radius: 50%;
  flex: none;
}

.mark.dirty {
  background: var(--accent);
}

.mark.invalid {
  background: var(--danger);
}

.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.head {
  flex: none;
  display: flex;
  align-items: center;
  padding: 12px 12px 12px 24px;
  border-bottom: 1px solid var(--border);
}

.head h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
}

.body {
  flex: 1;
  min-height: 0;
  padding: 4px 24px 16px;
}

.group {
  margin: 0;
  padding: 16px 0;
  border: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
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
