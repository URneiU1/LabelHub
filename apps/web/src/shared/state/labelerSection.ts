import { useSyncExternalStore } from 'react'

// 标注员左栏分节(任务广场 / 标注工作台 / 我的贡献)的高亮在 AppLayout 渲染,但实际显示的视图
// (任务广场列表 vs 作答页)由 LabelerPlaza 内部 view 状态决定 ——「继续标注 / 返回任务广场」只切
// 内部 view、不动 URL。两者分属不同子树(侧栏在 shell,页面在 <Outlet/>),用这个极简外部 store
// 桥接:LabelerPlaza setLabelerSection,AppLayout useLabelerSection 据此高亮,使左栏跟着实际内容走。
export type LabelerSection = 'tasks' | 'workbench' | 'mine'

const PATH_SECTION: Record<string, LabelerSection> = {
  '/labeler': 'tasks',
  '/labeler/workbench': 'workbench',
  '/labeler/mine': 'mine',
}

// 把侧栏导航项的 to 收敛回对应分节;非 labeler 项返回 undefined(侧栏据此回退到路由 isActive)。
export function labelerSectionForPath(path: string): LabelerSection | undefined {
  return PATH_SECTION[path]
}

let current: LabelerSection = 'tasks'
const listeners = new Set<() => void>()

export function getLabelerSection(): LabelerSection {
  return current
}

export function setLabelerSection(section: LabelerSection): void {
  if (section === current) {
    return
  }
  current = section
  listeners.forEach((listener) => listener())
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function useLabelerSection(): LabelerSection {
  return useSyncExternalStore(subscribe, getLabelerSection, getLabelerSection)
}
