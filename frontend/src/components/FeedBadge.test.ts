import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import FeedBadge from './FeedBadge.vue';
import { feedIconUrl } from '../api';
import { useReader } from '../stores/reader';
import { feed } from '../test/fakeBackend';

beforeEach(() => {
  setActivePinia(createPinia());
  const reader = useReader();
  reader.tree = {
    categories: [
      {
        id: 'user/-/label/Tech',
        label: 'Tech',
        feeds: [{ ...feed('feed/1', 'One'), icon_url: 'http://rss.lan/f.php?h=abc' }],
      },
    ],
    feeds: [
      feed('feed/2', 'Two'),
      { ...feed('feed/3', 'Three'), icon_url: 'http://rss.lan/f.php?h=gone' },
    ],
  };
});

describe('源徽标', () => {
  it('图标地址走本地 /api，带 iconUrl 作版本；没有 iconUrl 为空', () => {
    expect(feedIconUrl({ id: 'feed/1', icon_url: 'http://rss.lan/f.php?h=abc' })).toBe(
      '/api/feeds/icon?id=feed%2F1&v=http%3A%2F%2Frss.lan%2Ff.php%3Fh%3Dabc'
    );
    expect(feedIconUrl({ id: 'feed/2', icon_url: '' })).toBe('');
  });

  it('有 iconUrl 的源显示后端给的图标，不直连外部地址', () => {
    const w = mount(FeedBadge, { props: { id: 'feed/1', title: 'One' } });
    const img = w.find('img.fav');
    expect(img.attributes('src')).toMatch(/^\/api\/feeds\/icon\?id=feed%2F1&v=/);
    expect(img.attributes('alt')).toBe('');
    expect(w.find('span.fav').exists()).toBe(false);
  });

  it('没有 iconUrl 或不在订阅树里的源显示字母徽标', () => {
    for (const id of ['feed/2', 'feed/9']) {
      const w = mount(FeedBadge, { props: { id, title: 'Two' } });
      expect(w.find('img').exists()).toBe(false);
      expect(w.find('span.fav').text()).toBe('T');
    }
  });

  it('图标取不到时回落字母徽标，之后同一个源不再请求', async () => {
    const w = mount(FeedBadge, { props: { id: 'feed/3', title: 'Three' } });
    await w.find('img').trigger('error');
    expect(w.find('img').exists()).toBe(false);
    expect(w.find('span.fav').text()).toBe('T');

    const again = mount(FeedBadge, { props: { id: 'feed/3', title: 'Three' } });
    expect(again.find('img').exists()).toBe(false);
  });
});
