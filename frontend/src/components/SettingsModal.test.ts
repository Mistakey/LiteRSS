import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import SettingsModal from './SettingsModal.vue';
import { FakeBackend, settle } from '../test/fakeBackend';
import type { UpdateStatus } from '../api';

let be: FakeBackend;

beforeEach(() => {
  setActivePinia(createPinia());
  be = new FakeBackend();
  Object.assign(be.settings, {
    freshrss_server_url: 'http://nas:8080',
    freshrss_username: 'me',
    freshrss_api_password: 'stored-pass',
    llm_endpoint: 'https://llm.example.com/v1',
    llm_model: 'm1',
  });
  be.install();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function mounted() {
  const wrapper = mount(SettingsModal, { attachTo: document.body });
  await settle();
  return wrapper;
}

function updates() {
  return be.callsTo('POST', '/api/settings/update').map((c) => c.body);
}

describe('设置面板', () => {
  it('分组依次为 FreshRSS、摘要模型、标题翻译、网络代理、应用、关于', async () => {
    const w = await mounted();
    expect(w.findAll('legend').map((l) => l.text())).toEqual([
      'FreshRSS',
      '摘要模型',
      '标题翻译',
      '网络代理',
      '应用',
      '关于',
    ]);
    expect((w.find('[name=freshrss_server_url]').element as HTMLInputElement).value).toBe(
      'http://nas:8080'
    );
    w.unmount();
  });

  it('没改动时不能保存', async () => {
    const w = await mounted();
    expect(w.find('button[type=submit]').attributes('disabled')).toBeDefined();
    w.unmount();
  });

  it('保存只提交改过的键，已存凭据不回写，成功后关闭', async () => {
    const w = await mounted();
    await w.find('[name=freshrss_username]').setValue('you');
    await w.find('[name=freshrss_auto_sync_interval]').setValue('15');
    await w.find('[name=close_to_tray]').setValue(false);
    await w.find('form').trigger('submit');
    await settle();
    expect(updates()).toEqual([
      { freshrss_username: 'you', freshrss_auto_sync_interval: 15, close_to_tray: false },
    ]);
    expect(be.settings.freshrss_api_password).toBe('stored-pass');
    expect(w.emitted('close')).toHaveLength(1);
    w.unmount();
  });

  it('改回原值的键不提交', async () => {
    const w = await mounted();
    await w.find('[name=freshrss_username]').setValue('you');
    await w.find('[name=freshrss_username]').setValue('me');
    expect(w.find('button[type=submit]').attributes('disabled')).toBeDefined();
    w.unmount();
  });

  it('后端多给的键不在清单里，不会被提交', async () => {
    (be.settings as unknown as Record<string, unknown>).window_x = 5;
    const w = await mounted();
    await w.find('[name=llm_model]').setValue('m2');
    await w.find('form').trigger('submit');
    await settle();
    expect(updates()).toEqual([{ llm_model: 'm2' }]);
    w.unmount();
  });

  function secret(w: Awaited<ReturnType<typeof mounted>>, key: string) {
    return w.find(`[data-secret=${key}]`);
  }
  const buttons = (el: ReturnType<typeof secret>) => el.findAll('button').map((b) => b.text());
  function clears() {
    return be.callsTo('POST', '/api/settings/secrets/clear').map((c) => c.body);
  }

  it('已保存的密钥显示为不可编辑的掩码，带「修改」「清除」；未保存的是普通空输入框', async () => {
    const w = await mounted();
    const pass = secret(w, 'freshrss_api_password');
    const masked = pass.find('input').element as HTMLInputElement;
    expect(masked.readOnly).toBe(true);
    expect(masked.value).toBe('••••••••');
    expect(buttons(pass)).toEqual(['修改', '清除']);

    const key = secret(w, 'llm_api_key');
    const input = key.find('input').element as HTMLInputElement;
    expect(input.readOnly).toBe(false);
    expect(input.value).toBe('');
    expect(buttons(key)).toEqual([]);
    w.unmount();
  });

  it('修改：变成空的输入框，保存时写入新值', async () => {
    const w = await mounted();
    await secret(w, 'freshrss_api_password').find('button').trigger('click');
    const input = secret(w, 'freshrss_api_password').find('input');
    expect((input.element as HTMLInputElement).readOnly).toBe(false);
    expect((input.element as HTMLInputElement).value).toBe('');
    expect(buttons(secret(w, 'freshrss_api_password'))).toEqual(['取消']);
    await input.setValue('new-pass');
    await w.find('form').trigger('submit');
    await settle();
    expect(updates()).toEqual([{ freshrss_api_password: 'new-pass' }]);
    expect(be.settings.freshrss_api_password).toBe('new-pass');
    w.unmount();
  });

  it('修改后取消：恢复已保存的掩码，保存时不动原值', async () => {
    const w = await mounted();
    await secret(w, 'freshrss_api_password').find('button').trigger('click');
    await secret(w, 'freshrss_api_password').find('input').setValue('typo');
    await secret(w, 'freshrss_api_password').find('button').trigger('click');
    expect(
      (secret(w, 'freshrss_api_password').find('input').element as HTMLInputElement).value
    ).toBe('••••••••');
    expect(w.find('button[type=submit]').attributes('disabled')).toBeDefined();
    await w.find('[name=freshrss_username]').setValue('you');
    await w.find('form').trigger('submit');
    await settle();
    expect(updates()).toEqual([{ freshrss_username: 'you' }]);
    expect(be.settings.freshrss_api_password).toBe('stored-pass');
    w.unmount();
  });

  it('清除：可以撤销，保存时显式清除，不靠空串', async () => {
    be.settings.baidu_secret_key = 'old-key';
    const w = await mounted();
    const baidu = () => secret(w, 'baidu_secret_key');
    await baidu().findAll('button')[1].trigger('click');
    expect(baidu().text()).toContain('保存后清除');
    expect(buttons(baidu())).toEqual(['撤销']);
    await baidu().find('button').trigger('click');
    expect(buttons(baidu())).toEqual(['修改', '清除']);
    expect(w.find('button[type=submit]').attributes('disabled')).toBeDefined();

    await baidu().findAll('button')[1].trigger('click');
    await w.find('form').trigger('submit');
    await settle();
    expect(updates()).toEqual([]);
    expect(clears()).toEqual([{ key: 'baidu_secret_key' }]);
    expect(be.settings.baidu_secret_key).toBe('');
    expect(w.emitted('close')).toHaveLength(1);
    w.unmount();
  });

  it('未保存的密钥填了就提交', async () => {
    const w = await mounted();
    await secret(w, 'llm_api_key').find('input').setValue('sk-1');
    await w.find('form').trigger('submit');
    await settle();
    expect(updates()).toEqual([{ llm_api_key: 'sk-1' }]);
    expect(clears()).toEqual([]);
    w.unmount();
  });

  it('同步间隔不是正整数时不提交并提示', async () => {
    const w = await mounted();
    await w.find('[name=freshrss_auto_sync_interval]').setValue('0');
    await w.find('form').trigger('submit');
    await settle();
    expect(updates()).toEqual([]);
    expect(w.find('.save-error').text()).toContain('同步间隔');
    w.unmount();
  });

  it('后端拒收时显示原因，面板不关', async () => {
    const w = await mounted();
    await w.find('[name=freshrss_username]').setValue('you');
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response('invalid settings: proxy mode "manual" needs a host and port', { status: 400 })
    );
    await w.find('form').trigger('submit');
    await settle();
    expect(w.find('.save-error').text()).toContain('设置没有保存');
    expect(w.emitted('close')).toBeUndefined();
    w.unmount();
  });

  it('测试 FreshRSS 用表单里的值，没动的已存密码不传，显示成功结果', async () => {
    const w = await mounted();
    await w.find('[name=freshrss_server_url]').setValue('http://nas:9090');
    await w.find('[data-group=freshrss] .test .btn').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/settings/freshrss/test').map((c) => c.body)).toEqual([
      { freshrss_server_url: 'http://nas:9090', freshrss_username: 'me' },
    ]);
    const result = w.find('[data-test=freshrss]');
    expect(result.text()).toBe('连接成功。');
    expect(result.classes()).toContain('ok');
    // 测试不保存。
    expect(updates()).toEqual([]);
    w.unmount();
  });

  it('测试模型失败时显示后端给的原因', async () => {
    be.tests.llm = { ok: false, message: 'API 密钥不对，请检查。' };
    const w = await mounted();
    await w.find('[name=llm_api_key]').setValue('sk-new');
    await w.find('[data-group=llm] .test .btn').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/settings/llm/test').map((c) => c.body)).toEqual([
      { llm_endpoint: 'https://llm.example.com/v1', llm_model: 'm1', llm_api_key: 'sk-new' },
    ]);
    const result = w.find('[data-test=model]');
    expect(result.text()).toBe('API 密钥不对，请检查。');
    expect(result.classes()).toContain('fail');
    w.unmount();
  });

  it('测试请求本身失败时显示失败', async () => {
    const w = await mounted();
    vi.mocked(fetch).mockRejectedValueOnce(new TypeError('network'));
    await w.find('[data-group=freshrss] .test .btn').trigger('click');
    await settle();
    expect(w.find('[data-test=freshrss]').classes()).toContain('fail');
    w.unmount();
  });

  it('手动代理才显示地址与认证', async () => {
    const w = await mounted();
    expect(w.find('[name=proxy_host]').exists()).toBe(false);
    await w.find('[name=proxy_mode][value=manual]').setValue(true);
    expect(w.find('[name=proxy_host]').exists()).toBe(true);
    await w.find('[name=proxy_port]').setValue('1080');
    await w.find('form').trigger('submit');
    await settle();
    expect(updates()).toEqual([{ proxy_mode: 'manual', proxy_port: '1080' }]);
    w.unmount();
  });

  it('关于：显示版本，检查到新版本时可去发布页', async () => {
    be.update = { ...be.update, latest_version: '0.2.0', update_available: true };
    const w = await mounted();
    expect(w.find('.about').text()).toContain('0.1.0');
    await w.find('[data-group=about] .btn').trigger('click');
    await settle();
    expect(w.find('[data-test=update]').text()).toBe('有新版本 0.2.0。');
    await w.find('[data-group=about] .link').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/browser/open').map((c) => c.body)).toEqual([
      { url: be.update.release_url },
    ]);
    w.unmount();
  });

  it('关于：能在应用内更新时点一次「更新到 X」，面板显示下载进度直到安装', async () => {
    be.update = { ...be.update, latest_version: '0.2.0', update_available: true, in_app: true };
    const step = (state: UpdateStatus['state'], received = 0): UpdateStatus => ({
      state,
      version: '0.2.0',
      received,
      total: 4 * 1048576,
      message: '',
      release_url: '',
    });
    be.updateSteps = [
      step('downloading'),
      step('downloading', 1048576),
      step('verifying', 4 * 1048576),
      step('installing', 4 * 1048576),
    ];
    const w = await mounted();
    vi.useFakeTimers();
    await w.find('[data-group=about] .btn').trigger('click');
    await vi.advanceTimersByTimeAsync(0);
    expect(w.find('[data-test=update]').text()).toBe('有新版本 0.2.0。');
    expect(w.find('[data-group=about] .link').exists()).toBe(false);
    await w.find('[data-test=start-update]').trigger('click');
    await vi.advanceTimersByTimeAsync(0);
    expect(be.callsTo('POST', '/api/update/start')).toHaveLength(1);
    const text = () => w.find('[data-test=update-progress] .result').text();
    expect(text()).toBe('正在下载 0.2.0：0%（0.0 / 4.0 MB）');
    expect(w.find('[data-test=start-update]').exists()).toBe(false);
    await vi.advanceTimersByTimeAsync(500);
    expect(text()).toBe('正在下载 0.2.0：25%（1.0 / 4.0 MB）');
    expect((w.find('progress').element as HTMLProgressElement).value).toBe(1048576);
    await vi.advanceTimersByTimeAsync(500);
    expect(text()).toBe('正在校验安装包…');
    await vi.advanceTimersByTimeAsync(500);
    expect(text()).toBe('正在安装，LiteRSS 会退出并重新启动…');
    expect(w.find('[data-group=about] .btn').attributes('disabled')).toBeDefined();
    expect(be.callsTo('POST', '/api/update/start')).toHaveLength(1);
    vi.useRealTimers();
    w.unmount();
  });

  it('关于：更新失败时显示原因与发布页，开发构建显示不安装', async () => {
    be.update = { ...be.update, latest_version: '0.2.0', update_available: true, in_app: true };
    const failed: UpdateStatus = {
      state: 'failed',
      version: '0.2.0',
      received: 0,
      total: 0,
      message: '下载的安装包校验不符，已删除，请到发布页下载。',
      release_url: 'https://github.com/example/literss/releases/tag/v0.2.0',
    };
    be.updateSteps = [{ ...failed, state: 'downloading', message: '', release_url: '' }, failed];
    const w = await mounted();
    await w.find('[data-group=about] .btn').trigger('click');
    await settle();
    await w.find('[data-test=start-update]').trigger('click');
    await new Promise((r) => setTimeout(r, 600));
    await settle();
    const progress = w.find('[data-test=update-progress]');
    expect(progress.find('[role=alert]').text()).toBe(failed.message);
    await progress.find('.link').trigger('click');
    await settle();
    expect(be.callsTo('POST', '/api/browser/open').map((c) => c.body)).toEqual([
      { url: failed.release_url },
    ]);
    w.unmount();

    // 面板打开时已有进行过的更新（例如托盘开始的）就直接显示。
    be.updateSteps = [
      {
        ...failed,
        state: 'not_installed',
        message: '开发构建不安装：安装包已下载并通过校验。',
        release_url: '',
      },
    ];
    await fetch('/api/update/start', { method: 'POST' });
    const again = await mounted();
    expect(again.find('[data-test=update-progress] [role=status]').text()).toBe(
      '开发构建不安装：安装包已下载并通过校验。'
    );
    again.unmount();
  });

  it('Esc、取消与点背景都关闭且不保存', async () => {
    const w = await mounted();
    await w.find('[name=freshrss_username]').setValue('you');
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await w.findAll('.foot .btn')[0].trigger('click');
    await w.find('.mask').trigger('mousedown');
    expect(w.emitted('close')).toHaveLength(3);
    expect(updates()).toEqual([]);
    w.unmount();
  });
});
