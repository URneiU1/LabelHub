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

// 子侧栏:进入某个分节后,若该分节有多个子区域,在父项下面展开可跳转的子项。
// anchor 对应面板内元素的 id(见 Dashboard / 各子面板);label 为显示文案。
// 只给「真有多个子区域」的分节配子项;任务管理/模板搭建是单一视图,不展开。
// 增删子项只改这一处即可。
export interface OwnerSubItem {
  anchor: string
  label: string
}

export const OWNER_SUB_NAV: Partial<Record<OwnerSection, ReadonlyArray<OwnerSubItem>>> = {
  ai: [
    { anchor: 'ai-baseline', label: 'Baseline 说明' },
    { anchor: 'ai-prompts', label: 'Prompt 配置' },
    { anchor: 'ai-dryrun', label: 'Dry-run 测试' },
    { anchor: 'ai-golden', label: '评测集' },
    { anchor: 'ai-history', label: '试跑历史' },
  ],
  dataset: [
    { anchor: 'ds-file', label: '文件导入' },
    { anchor: 'ds-json', label: 'JSON 导入' },
    { anchor: 'ds-items', label: '题目列表' },
  ],
  review: [
    { anchor: 'rv-summary', label: '审核汇总' },
    { anchor: 'rv-results', label: '逐条质检结果' },
  ],
  stats: [
    { anchor: 'st-progress', label: '进度' },
    { anchor: 'st-pass', label: '通过率' },
    { anchor: 'st-status', label: '状态分布' },
    { anchor: 'st-dim', label: '维度均分' },
  ],
  export: [
    { anchor: 'ex-config', label: '导出配置' },
    { anchor: 'ex-history', label: '导出历史' },
  ],
}

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

// 子项点击 → 滚动目标。seq 让重复点击同一锚点也能再次触发滚动。
export interface OwnerSubTarget {
  anchor: string
  seq: number
}

let subTargetSeq = 0
let currentSubTarget: OwnerSubTarget | null = null
const subTargetListeners = new Set<() => void>()

export function getOwnerSubTarget(): OwnerSubTarget | null {
  return currentSubTarget
}

export function setOwnerSubTarget(anchor: string): void {
  subTargetSeq += 1
  currentSubTarget = { anchor, seq: subTargetSeq }
  subTargetListeners.forEach((listener) => listener())
}

// 测试用:重置滚动目标,避免模块级状态跨用例泄漏。
export function resetOwnerSubTarget(): void {
  currentSubTarget = null
  subTargetListeners.forEach((listener) => listener())
}

function subscribeSubTarget(listener: () => void): () => void {
  subTargetListeners.add(listener)
  return () => {
    subTargetListeners.delete(listener)
  }
}

export function useOwnerSubTarget(): OwnerSubTarget | null {
  return useSyncExternalStore(subscribeSubTarget, getOwnerSubTarget, getOwnerSubTarget)
}
