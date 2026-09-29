import { describe, expect, it } from 'vitest';
import { settingsDefaults } from '../types/settings.generated';
import { READER_KEYS, SETTING_KEYS } from './settings';

describe('设置键清单', () => {
  it('每个生成的键恰好在面板分组或 reader 分组里出现一次', () => {
    const listed = [...SETTING_KEYS, ...READER_KEYS];
    expect(new Set(listed).size).toBe(listed.length);
    expect([...listed].sort()).toEqual(Object.keys(settingsDefaults).sort());
  });
});
