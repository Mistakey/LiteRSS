import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FakeBackend } from '../test/fakeBackend';
import { SNACKBAR_MS, useSnackbar } from './snackbar';

let be: FakeBackend;

beforeEach(() => {
  setActivePinia(createPinia());
  be = new FakeBackend();
  be.install();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('撤销 snackbar', () => {
  it('约 8 秒后消失', () => {
    const s = useSnackbar();
    s.show({ text: '3 篇已标为已读', token: 't' });
    vi.advanceTimersByTime(SNACKBAR_MS - 1);
    expect(s.current).not.toBeNull();
    vi.advanceTimersByTime(1);
    expect(s.current).toBeNull();
  });

  it('悬停时不消失，离开后重新计时', () => {
    const s = useSnackbar();
    s.show({ text: 'x', token: 't' });
    vi.advanceTimersByTime(SNACKBAR_MS - 100);
    s.hold();
    vi.advanceTimersByTime(SNACKBAR_MS * 3);
    expect(s.current).not.toBeNull();
    s.release();
    vi.advanceTimersByTime(SNACKBAR_MS);
    expect(s.current).toBeNull();
  });

  it('令牌过期（410）时直接消失，不调回调', async () => {
    const s = useSnackbar();
    const onUndone = vi.fn();
    s.show({ text: 'x', token: '不存在', onUndone });
    await s.undo();
    expect(be.callsTo('POST', '/api/undo')[0].body).toEqual({ token: '不存在' });
    expect(s.current).toBeNull();
    expect(onUndone).not.toHaveBeenCalled();
  });

  it('其他失败时提示重试并保留令牌', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('boom', { status: 500 }))
    );
    const s = useSnackbar();
    s.show({ text: 'x', token: 't' });
    await s.undo();
    expect(s.current?.text).toBe('撤销失败，请重试');
    expect(s.current?.token).toBe('t');
  });
});
