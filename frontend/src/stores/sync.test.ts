import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SyncState } from '../api';
import { FakeBackend, settle } from '../test/fakeBackend';
import { syncLabel, useSync } from './sync';

let be: FakeBackend;

const base: SyncState = {
  rev: 1,
  running: false,
  new_items: 0,
  pending: 0,
  last_sync_at: 1000,
  error: '',
};

beforeEach(() => {
  setActivePinia(createPinia());
  be = new FakeBackend();
  be.install();
});

afterEach(() => {
  useSync().stop();
  vi.unstubAllGlobals();
});

describe('同步状态长轮询', () => {
  it('以上次的 rev 继续等待，周期结束时回调', async () => {
    be.syncStates = [
      base,
      { ...base, rev: 2, running: true },
      { ...base, rev: 3, running: false, last_sync_at: 2000 },
    ];
    const onCycle = vi.fn();
    const sync = useSync();
    sync.start(onCycle);
    await settle();

    const sinces = be.calls.filter((c) => c.path === '/api/sync/state');
    expect(sinces).toHaveLength(4);
    expect(sync.state?.rev).toBe(3);
    expect(onCycle).toHaveBeenCalledTimes(1);
  });

  it('请求带上 since', async () => {
    be.syncStates = [base];
    const fetchMock = vi.mocked(fetch);
    useSync().start(() => {});
    await settle();
    const urls = fetchMock.mock.calls.map((c) => String(c[0]));
    expect(urls).toEqual(['/api/sync/state?since=0', '/api/sync/state?since=1']);
  });

  it('首个状态不算周期结束', async () => {
    be.syncStates = [base];
    const onCycle = vi.fn();
    useSync().start(onCycle);
    await settle();
    expect(onCycle).not.toHaveBeenCalled();
  });
});

describe('同步状态文案', () => {
  const now = 1_000_000;
  it.each([
    [null, '正在连接…'],
    [{ ...base, running: true }, '正在同步…'],
    [{ ...base, error: 'sync: the FreshRSS account is not configured' }, '未配置 FreshRSS 账号'],
    [{ ...base, error: 'dial tcp: refused' }, '同步失败'],
    [{ ...base, last_sync_at: 0 }, '尚未同步'],
    [{ ...base, last_sync_at: now - 30 }, '刚刚已同步'],
    [{ ...base, last_sync_at: now - 5 * 60 }, '5 分钟前已同步'],
    [{ ...base, last_sync_at: now - 3 * 3600 }, '3 小时前已同步'],
  ])('%j → %s', (s, want) => {
    expect(syncLabel(s as SyncState | null, now)).toBe(want);
  });
});
