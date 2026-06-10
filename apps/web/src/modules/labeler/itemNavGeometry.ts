// 题目导航虚拟滚动的定高几何:行高 + overscan + 可视区间计算。
// 从 ItemNav.tsx 拆出,使组件文件只导出组件(满足 react-refresh/only-export-components)。

// 单行行高:.wb-side__item 固定 38px + margin-bottom 2px(见 workbench.css)。虚拟滚动按此定高切片。
export const ITEM_ROW_HEIGHT = 40
const OVERSCAN = 8

// visibleRange 计算定高列表在当前滚动位置下应渲染的 [start, end)(含 overscan 缓冲)。
// viewportHeight<=0(未测量 / 测试 / SSR)时返回整段,优雅回退为「渲染全部」,保证题目始终可达。
export function visibleRange(
  scrollTop: number,
  viewportHeight: number,
  rowHeight: number,
  total: number,
  overscan = OVERSCAN,
): { start: number, end: number } {
  if (total <= 0) {
    return { start: 0, end: 0 }
  }
  if (viewportHeight <= 0 || rowHeight <= 0) {
    return { start: 0, end: total }
  }
  const first = Math.floor(scrollTop / rowHeight)
  const visibleCount = Math.ceil(viewportHeight / rowHeight)
  const start = Math.max(0, first - overscan)
  const end = Math.min(total, first + visibleCount + overscan)
  return { start, end }
}
