import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api';
import { FakeBackend, settle } from '../test/fakeBackend';
import { usePrefs } from './prefs';
import { useSnackbar } from './snackbar';

let be: FakeBackend;

beforeEach(() => {
  setActivePinia(createPinia());
  be = new FakeBackend();
  be.install();
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('prefs', () => {
  it('从设置读出 Bionic Reading 开关', async () => {
    be.settings.bionic_reading = true;
    const prefs = usePrefs();
    expect(prefs.bionic).toBe(false);
    await settle();
    expect(prefs.bionic).toBe(true);
  });

  it('读到之前用户已经点过：以用户点的为准', async () => {
    be.settings.bionic_reading = false;
    const prefs = usePrefs();
    void prefs.toggleBionic();
    await settle();
    expect(prefs.bionic).toBe(true);
    expect(be.settings.bionic_reading).toBe(true);
  });

  it('保存失败：本次照样生效，提示下次会恢复', async () => {
    const prefs = usePrefs();
    await settle();
    vi.spyOn(api, 'updateSettings').mockRejectedValue(new Error('down'));
    await prefs.toggleBionic();
    expect(prefs.bionic).toBe(true);
    expect(useSnackbar().current?.text).toContain('下次打开会恢复原样');
  });
});
