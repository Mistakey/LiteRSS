/**
 * 图片查看器的几何（spec D15）：图片左上角在 (x, y)，缩放为 scale，都是相对查看器容器的像素。
 */
export interface View {
  scale: number;
  x: number;
  y: number;
}

export interface Size {
  width: number;
  height: number;
}

/** 四周留白，适应窗口时图片不贴边。 */
const MARGIN = 48;
export const MAX_SCALE = 8;

/** 适应窗口的缩放：放得下时用原始大小，不放大。 */
export function fitScale(image: Size, box: Size): number {
  const w = Math.max(box.width - 2 * MARGIN, 1);
  const h = Math.max(box.height - 2 * MARGIN, 1);
  return Math.min(1, w / image.width, h / image.height);
}

export function centered(image: Size, box: Size, scale: number): View {
  return {
    scale,
    x: (box.width - image.width * scale) / 2,
    y: (box.height - image.height * scale) / 2,
  };
}

/** 以 (px, py) 为中心缩放到 scale：指针下的那一点保持不动。 */
export function zoomAt(view: View, scale: number, px: number, py: number, min: number): View {
  const s = Math.min(MAX_SCALE, Math.max(min, scale));
  const k = s / view.scale;
  return { scale: s, x: px - (px - view.x) * k, y: py - (py - view.y) * k };
}

/** 图片比容器大时才能拖动平移。 */
export function draggable(image: Size, box: Size, view: View): boolean {
  return image.width * view.scale > box.width || image.height * view.scale > box.height;
}
