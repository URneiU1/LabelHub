import { useSyncExternalStore } from 'react'

// Owner 后台的左栏分节由全局「工作区」侧栏(AppLayout)驱动,但分节内容渲染在
// Owner 页(Dashboard)。两个组件分属不同子树(侧栏在 shell,页面在 <Outlet/>),
// 用这个极简外部 store 桥接:侧栏 setOwnerSection,页面 useOwnerSection 订阅。
export type OwnerSection = 'tasks' | 'template' | 'dataset' | 'ai' | 'review' | 'stats' | 'export'

export interface OwnerNavGroup {
  title: string
  items: ReadonlyArray<readonly [OwnerSection, string]>
}

// 对齐组织方参考稿 ~/labelhub-ui-demo 的 SideNav 三组结构。
// 「任务管理」= 任务列表 / 新建 / 状态机(TaskManagePanel);人工审核「动作」归
// Reviewer 角色,Owner 的「审核结果」只读。
export const OWNER_NAV_GROUPS: ReadonlyArray<OwnerNavGroup> = [
  { title: '数据生产', items: [['tasks', '任务管理'], ['template', '模板搭建'], ['dataset', '数据集']] },
  { title: '审核与质检', items: [['ai', 'AI 预审'], ['review', '审核结果']] },
  { title: '数据交付', items: [['stats', '数据看板'], ['export', '数据导出']] },
]

const DEFAULT_SECTION: OwnerSection = 'ai'

let currentSection: OwnerSection = DEFAULT_SECTION
const listeners = new Set<() => void>()

export function getOwnerSection(): OwnerSection {
  return currentSection
}

export function setOwnerSection(section: OwnerSection): void {
  if (section === currentSection) return
  currentSection = section
  listeners.forEach((listener) => listener())
}

// 测试用:把分节重置回默认,避免模块级状态跨用例泄漏。
export function resetOwnerSection(): void {
  setOwnerSection(DEFAULT_SECTION)
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function useOwnerSection(): OwnerSection {
  return useSyncExternalStore(subscribe, getOwnerSection, getOwnerSection)
}
