/**
 * 内联图标，取自布局原型（24×24 线条图标）。每个图标是一组 SVG 子元素，
 * 由 Icon 组件渲染，不经 v-html。
 */
export type IconShape = readonly [tag: 'path' | 'circle' | 'rect', attrs: Record<string, string>];

const path = (d: string): IconShape => ['path', { d }];
const circle = (cx: string, cy: string, r: string): IconShape => ['circle', { cx, cy, r }];

export const icons = {
  all: [['rect', { x: '3', y: '4', width: '18', height: '16', rx: '2' }], path('M3 9h18')],
  gear: [
    circle('12', '12', '3'),
    path(
      'M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z'
    ),
  ],
  ext: [
    path('M14 4h6v6'),
    path('M20 4l-9 9'),
    path('M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5'),
  ],
  spark: [
    path('M12 3l1.8 4.9L19 9.7l-5.2 1.8L12 16.5l-1.8-5L5 9.7l5.2-1.8z'),
    path('M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8z'),
  ],
  translate: [
    path('M4 5h9M8.5 3v2M11 5c-1 4-3.5 7-7 9M6.5 8.5c1.2 2 2.8 3.6 5 4.8'),
    path('M13 21l4-9 4 9M14.5 17.5h5'),
  ],
  mailOpen: [path('M3 10l9-6 9 6v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z'), path('M3 10l9 6 9-6')],
  mail: [['rect', { x: '3', y: '5', width: '18', height: '14', rx: '1' }], path('M3 7l9 6 9-6')],
  refresh: [
    path('M20 11a8 8 0 0 0-14.3-4.9L4 8'),
    path('M4 4v4h4'),
    path('M4 13a8 8 0 0 0 14.3 4.9L20 16'),
    path('M20 20v-4h-4'),
  ],
  cloud: [path('M7 18a5 5 0 1 1 .9-9.9A6 6 0 0 1 19 10a4 4 0 0 1-1 8z')],
  chev: [path('M6 9l6 6 6-6')],
  x: [path('M6 6l12 12M18 6L6 18')],
  zin: [circle('11', '11', '7'), path('M21 21l-4.3-4.3M8 11h6M11 8v6')],
  zout: [circle('11', '11', '7'), path('M21 21l-4.3-4.3M8 11h6')],
  left: [path('M15 6l-6 6 6 6')],
  right: [path('M9 6l6 6-6 6')],
  up: [path('M12 19V5M6 11l6-6 6 6')],
  down: [path('M12 5v14M6 13l6 6 6-6')],
  check: [path('M5 12l5 5L20 7')],
  checks: [path('M2 12l5 5L17 7'), path('M12 16l1 1L23 7')],
  warn: [path('M12 3l10 18H2z'), path('M12 10v5M12 18v.01')],
  // 顶栏的窗口按钮，仿 Windows 标题栏的细线字形
  winMin: [path('M3 12h18')],
  winMax: [['rect', { x: '3', y: '3', width: '18', height: '18' }]],
  winRestore: [['rect', { x: '3', y: '7', width: '14', height: '14' }], path('M7 7V3h14v14h-4')],
  winClose: [path('M3 3l18 18M21 3L3 21')],
} satisfies Record<string, readonly IconShape[]>;

export type IconName = keyof typeof icons;
