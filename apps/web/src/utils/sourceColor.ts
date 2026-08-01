/**
 * Source Identity Color
 *
 * 为每个订阅源生成稳定的低饱和度色相，避免高饱和破坏整体氛围。
 * 同一来源在不同会话、不同设备下颜色保持一致。
 */

export function sourceHue(name: string): number {
  let hash = 0;
  for (const char of name) {
    hash = (hash * 31 + char.charCodeAt(0)) % 360;
  }
  // 避开接近红色的危险区域（0-15 与 345-360），让颜色更偏向自然/编辑感
  if (hash < 15) hash += 25;
  if (hash > 345) hash -= 25;
  return hash;
}

export function sourceColor(
  name: string,
  mode: 'light' | 'dark' = 'light',
  options: { lightness?: number; chroma?: number } = {}
): string {
  const h = sourceHue(name);
  const l = options.lightness ?? (mode === 'light' ? 0.58 : 0.68);
  const c = options.chroma ?? (mode === 'light' ? 0.1 : 0.08);
  return `oklch(${l} ${c} ${h})`;
}

/**
 * 返回一个用于暗色/亮色模式下可读性较好的颜色字符串。
 * 通过检测当前 html 的 data-theme 属性自动判断模式。
 */
export function sourceColorAuto(name: string): string {
  const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
  return sourceColor(name, isDark ? 'dark' : 'light');
}
