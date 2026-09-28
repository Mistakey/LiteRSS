/**
 * 同步状态：长轮询 `GET /api/sync/state?since=<rev>`（spec D14）。每个周期结束（运行中 → 停下，
 * 或上次成功时间变了）调用 start 传入的回调，由它刷新侧栏与列表。
 * rev 不随每次意图写入变化（ARCHITECTURE「同步」），自己操作后的计数由阅读 store 自行刷新。
 */
import { defineStore } from 'pinia';
import { shallowRef } from 'vue';
import { api, type SyncState } from '../api';

/** 长轮询失败后的重试间隔。 */
export const RETRY_MS = 5000;

export const useSync = defineStore('sync', () => {
  const state = shallowRef<SyncState | null>(null);
  let controller: AbortController | null = null;

  function start(onCycleDone: () => void | Promise<void>) {
    if (controller) return;
    const ctl = new AbortController();
    controller = ctl;
    void (async () => {
      while (!ctl.signal.aborted) {
        try {
          const next = await api.syncState(state.value?.rev ?? 0, ctl.signal);
          const prev = state.value;
          state.value = next;
          if (
            prev &&
            ((prev.running && !next.running) || prev.last_sync_at !== next.last_sync_at)
          ) {
            await onCycleDone();
          }
        } catch {
          if (ctl.signal.aborted) return;
          await new Promise((r) => setTimeout(r, RETRY_MS));
        }
      }
    })();
  }

  function stop() {
    controller?.abort();
    controller = null;
  }

  /** 点侧栏底部的同步状态：请后端立即跑一轮，进度由长轮询报告。 */
  async function runNow() {
    try {
      await api.syncNow();
    } catch {
      // 后端不可达时长轮询也会失败并重试，状态文案不变。
    }
  }

  return { state, start, stop, runNow };
});

const NOT_CONFIGURED = 'the FreshRSS account is not configured';

/** 侧栏底部的同步状态文案；now 为 Unix 秒。 */
export function syncLabel(s: SyncState | null, now: number): string {
  if (!s) return '正在连接…';
  if (s.running) return '正在同步…';
  // 错误串带调用链前缀（如 "sync: …"），按结尾判断。
  if (s.error.endsWith(NOT_CONFIGURED)) return '未配置 FreshRSS 账号';
  if (s.error) return '同步失败，点击重试';
  if (!s.last_sync_at) return '尚未同步';
  const min = Math.floor((now - s.last_sync_at) / 60);
  if (min < 1) return '刚刚已同步';
  if (min < 60) return `${min} 分钟前已同步`;
  if (min < 1440) return `${Math.floor(min / 60)} 小时前已同步`;
  return `${Math.floor(min / 1440)} 天前已同步`;
}
