import { mount } from '@vue/test-utils';
import { afterEach, describe, expect, it } from 'vitest';
import { nextTick } from 'vue';
import ImageViewer from './ImageViewer.vue';

afterEach(() => {
  document.body.innerHTML = '';
});

describe('ImageViewer', () => {
  it('相邻两张同一地址的图也换新元素，重新等待载入', async () => {
    const w = mount(ImageViewer, {
      props: { images: ['https://img.example/a.png', 'https://img.example/a.png'], start: 0 },
      attachTo: document.body,
    });
    const first = document.querySelector('.viewer img')!;
    first.dispatchEvent(new Event('load'));
    await nextTick();
    expect(document.querySelector('.viewer .zoom')).not.toBeNull();

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }));
    await nextTick();
    const second = document.querySelector('.viewer img')!;
    expect(second).not.toBe(first);
    expect(document.querySelector('.viewer .count')!.textContent).toBe('2 / 2');
    second.dispatchEvent(new Event('load'));
    await nextTick();
    expect(document.querySelector('.viewer .zoom')).not.toBeNull();
    w.unmount();
  });

  it('载入失败时给出提示', async () => {
    const w = mount(ImageViewer, {
      props: { images: ['https://img.example/broken.png'], start: 0 },
      attachTo: document.body,
    });
    document.querySelector('.viewer img')!.dispatchEvent(new Event('error'));
    await nextTick();
    expect(document.querySelector('.viewer .failed')!.textContent).toBe('图片载入失败');
    w.unmount();
  });
});
