/**
 * 阅读偏好：目前只有 Bionic Reading 开关（spec D22）。
 * 存在设置清单的 reader 分组（bionic_reading），经设置接口读写、跨会话记住，不在设置面板显示。
 * 读取失败按默认的关；用户在读到之前就点过开关时，以用户点的为准。
 */
import { defineStore } from 'pinia';
import { ref } from 'vue';
import { api } from '../api';
import { useSnackbar } from './snackbar';

export const usePrefs = defineStore('prefs', () => {
  const bionic = ref(false);
  let touched = false;

  async function load() {
    try {
      const view = await api.settings();
      if (!touched) bionic.value = view.settings.bionic_reading;
    } catch {
      // 保持默认的关
    }
  }

  /** 切换并保存；保存失败时本次会话照样生效，提示下次打开会恢复原样。 */
  async function toggleBionic() {
    touched = true;
    const next = !bionic.value;
    bionic.value = next;
    try {
      await api.updateSettings({ bionic_reading: next });
    } catch {
      useSnackbar().show({ text: '没能记住 Bionic Reading 开关，下次打开会恢复原样' });
    }
  }

  void load();

  return { bionic, toggleBionic };
});
