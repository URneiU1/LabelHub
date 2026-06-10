import { describe, expect, it } from 'vitest'
import { visibleRange, ITEM_ROW_HEIGHT } from './ItemNav'

// 虚拟滚动核心是定高切片的纯函数 visibleRange;DOM 渲染交给浏览器手验,这里锁住区间逻辑。
describe('visibleRange', () => {
  it('renders only the visible window plus overscan for a large list', () => {
    // 视口 400px / 行 40px = 10 行可见;overscan 8。滚到顶部。
    const { start, end } = visibleRange(0, 400, ITEM_ROW_HEIGHT, 5000)
    expect(start).toBe(0)
    expect(end).toBe(18) // 10 可见 + 8 下缓冲
    // 5000 题里只渲染 18 行 —— 这正是把 DOM 从线性压到常数的关键。
    expect(end - start).toBeLessThan(30)
  })

  it('windows around the current scroll position with overscan on both sides', () => {
    // 滚到第 2500 行附近(scrollTop = 2500*40)。
    const { start, end } = visibleRange(2500 * ITEM_ROW_HEIGHT, 400, ITEM_ROW_HEIGHT, 5000)
    expect(start).toBe(2500 - 8) // first(2500) - overscan
    expect(end).toBe(2500 + 10 + 8) // first + visibleCount + overscan
  })

  it('clamps the window to list bounds at the tail', () => {
    const { start, end } = visibleRange(4999 * ITEM_ROW_HEIGHT, 400, ITEM_ROW_HEIGHT, 5000)
    expect(end).toBe(5000) // 不越界
    expect(start).toBeGreaterThanOrEqual(0)
  })

  it('falls back to rendering everything when the viewport is not yet measured', () => {
    // viewportHeight=0(测试 / SSR / 首帧未测量)→ 渲染全部,题目始终可达,不会白屏。
    expect(visibleRange(0, 0, ITEM_ROW_HEIGHT, 5000)).toEqual({ start: 0, end: 5000 })
  })

  it('handles an empty list', () => {
    expect(visibleRange(0, 400, ITEM_ROW_HEIGHT, 0)).toEqual({ start: 0, end: 0 })
  })
})
