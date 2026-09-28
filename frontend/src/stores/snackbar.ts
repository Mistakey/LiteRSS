/**
 * 撤销 snackbar（spec D7、D15）：窗口左下角，约 8 秒后消失，悬停时不消失；所有批量已读共用。
 * 撤销调用后端令牌，令牌过期或已用（410）时直接消失。
 */
import { defineStore } from 'pinia';
import { ref, shallowRef } from 'vue';
import { api, ApiError } from '../api';

export const SNACKBAR_MS = 8000;

export interface SnackbarMessage {
  text: string;
  /** 批量已读的撤销令牌；没有时不显示「撤销」。 */
  token?: string;
  /** 撤销成功后调用，用来刷新列表与计数。 */
  onUndone?: () => void | Promise<void>;
}

export const useSnackbar = defineStore('snackbar', () => {
  const current = shallowRef<SnackbarMessage | null>(null);
  const undoing = ref(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function arm() {
    clearTimeout(timer);
    timer = setTimeout(dismiss, SNACKBAR_MS);
  }

  function show(msg: SnackbarMessage) {
    current.value = msg;
    undoing.value = false;
    arm();
  }

  function dismiss() {
    clearTimeout(timer);
    current.value = null;
  }

  /** 悬停时停表，离开后重新计时。 */
  function hold() {
    clearTimeout(timer);
  }

  function release() {
    if (current.value) arm();
  }

  async function undo() {
    const msg = current.value;
    if (!msg?.token || undoing.value) return;
    undoing.value = true;
    try {
      await api.undo(msg.token);
      if (current.value === msg) dismiss();
      await msg.onUndone?.();
    } catch (err) {
      if (current.value !== msg) return;
      if (err instanceof ApiError && err.status === 410) dismiss();
      // 令牌在后端还有效（约 60 秒），保留「撤销」让用户重试。
      else show({ ...msg, text: '撤销失败，请重试' });
    } finally {
      undoing.value = false;
    }
  }

  return { current, undoing, show, dismiss, hold, release, undo };
});
