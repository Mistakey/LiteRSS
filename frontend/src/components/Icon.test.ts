import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import Icon from './Icon.vue';
import { icons, type IconName } from './icons';

const SVG_NS = 'http://www.w3.org/2000/svg';

describe('Icon', () => {
  it.each(Object.keys(icons) as IconName[])('%s 渲染为 SVG 命名空间下的图形', (name) => {
    const svg = mount(Icon, { props: { name } }).element as SVGElement;
    expect(svg.namespaceURI).toBe(SVG_NS);
    expect(svg.children).toHaveLength(icons[name].length);
    for (const child of Array.from(svg.children)) {
      expect(child.namespaceURI).toBe(SVG_NS);
    }
  });

  it('按形状写入属性', () => {
    const svg = mount(Icon, { props: { name: 'zin' } }).element;
    const circle = svg.querySelector('circle');
    expect(circle?.getAttribute('r')).toBe('7');
    expect(svg.querySelector('path')?.getAttribute('d')).toBe('M21 21l-4.3-4.3M8 11h6M11 8v6');
  });
});
