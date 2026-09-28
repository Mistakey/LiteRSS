import { describe, expect, it } from 'vitest';
import { centered, draggable, fitScale, MAX_SCALE, zoomAt } from './viewer';

const box = { width: 1096, height: 896 };

describe('fitScale', () => {
  it('大图缩到窗口内（四周留白 48）', () => {
    expect(fitScale({ width: 2000, height: 800 }, box)).toBe(0.5);
  });

  it('小图保持原始大小，不放大', () => {
    expect(fitScale({ width: 300, height: 200 }, box)).toBe(1);
  });
});

describe('zoomAt', () => {
  it('指针下的点保持不动', () => {
    const v = centered({ width: 2000, height: 800 }, box, 0.5);
    const z = zoomAt(v, 1, 300, 400, 0.25);
    // 缩放前后，指针 (300, 400) 对应的图片坐标相同
    expect((300 - v.x) / v.scale).toBeCloseTo((300 - z.x) / z.scale);
    expect((400 - v.y) / v.scale).toBeCloseTo((400 - z.y) / z.scale);
  });

  it('缩放限制在上下限之间', () => {
    const v = { scale: 1, x: 0, y: 0 };
    expect(zoomAt(v, 100, 0, 0, 0.5).scale).toBe(MAX_SCALE);
    expect(zoomAt(v, 0.01, 0, 0, 0.5).scale).toBe(0.5);
  });
});

describe('draggable', () => {
  it('图片超出窗口才能拖动', () => {
    const img = { width: 2000, height: 800 };
    expect(draggable(img, box, centered(img, box, 0.5))).toBe(false);
    expect(draggable(img, box, centered(img, box, 1))).toBe(true);
  });
});
