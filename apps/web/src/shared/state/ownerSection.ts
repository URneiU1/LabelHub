import { useSyncExternalStore } from 'react'

// Owner 后台的左栏分节由全局「工作区」侧栏(AppLayout)驱动,但分节内容渲染在
// Owner 页(Dashboard)。两个组件分属不同子树(侧栏在 shell,页面在 <Outlet/>),
// 用这个极简外部 store 桥接:侧栏 setOwnerSection,页面 useOwnerSection 订阅。
export type OwnerSection = 'tasks' | 'template' | 'dataset' | 'ai' | 'review' | 'acceptance' | 'stats' | 'export'

export interface OwnerNavGroup {
  title: string
  items: ReadonlyArray<readonly [OwnerSection, string]>
}

// 对齐组织方参考稿 ~/labelhub-ui-demo 的 SideNav 三组结构。
// 「任务管理」= 任务列表 / 新建 / 状态机(TaskManagePanel);人工审核「动作」归
// Reviewer 角色,Owner 的「审核结果」只读。
export const OWNER_NAV_GROUPS: ReadonlyArray<OwnerNavGroup> = [
  { title: '数据生产', items: [['tasks', '任务管理'], ['template', '模板搭建'], ['dataset', '数据集']] },
  { title: '质量控制', items: [['ai', 'AI 预审配置'], ['review', '审核质检']] },
  { title: '数据交付', items: [['acceptance', '数据验收'], ['stats', '生产看板'], ['export', '数据导出']] },
]

const OWNER_SECTION_KEYS: ReadonlyArray<OwnerSection> = ['tasks', 'template', 'dataset', 'ai', 'review', 'acceptance', 'stats', 'export']

// 路由 param 校验:把 /owner/:section 的字符串收敛回合法分节,否则回退默认。
export function isOwnerSection(value: string | undefined): value is OwnerSection {
  return value !== undefined && (OWNER_SECTION_KEYS as ReadonlyArray<string>).includes(value)
}

// 页内标签页(子页面):只有「AI 预审」分节内容多到需要拆子页;其它分节是单一页面。
// 子页 URL 形如 /owner/ai/<key>;key 同时用于 Dashboard 内 CSS data-sub 过滤显示。
// 增删子页只改这一处即可。
export interface OwnerSubItem {
  key: string
  label: string
}

export const OWNER_SUB_NAV: Partial<Record<OwnerSection, ReadonlyArray<OwnerSubItem>>> = {
  ai: [
    { key: 'prompts', label: 'Prompt 配置' },
    { key: 'baseline', label: 'Baseline 说明' },
    { key: 'dryrun', label: 'Dry-run 测试' },
    { key: 'golden', label: '评测集' },
    { key: 'history', label: '试跑历史' },
  ],
  export: [
    { key: 'config', label: '导出配置' },
    { key: 'history', label: '导出历史' },
  ],
}

// 这些分节的子页是「互斥二选一」,不需要「全部/概览」标签:不渲染「全部」,
// 进入分节直接默认落到这里指定的第一个子页(在 App.tsx 配重定向)。
// 不在此表的分节(如 AI 预审,工具多)保留「全部」概览入口。
export const SECTION_DEFAULT_SUB: Partial<Record<OwnerSection, string>> = {
  export: 'config',
}

export const DEFAULT_SECTION: OwnerSection = 'tasks'

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

// 当前子页(/owner/ai/<sub> 的 sub);null = 该分节的「全部/概览」。
// 由 AppLayout 从 URL 同步写入,Dashboard 订阅后用于 CSS data-sub 过滤显示。
let currentSubView: string | null = null
const subViewListeners = new Set<() => void>()

export function getOwnerSubView(): string | null {
  return currentSubView
}

export function setOwnerSubView(sub: string | null): void {
  if (sub === currentSubView) return
  currentSubView = sub
  subViewListeners.forEach((listener) => listener())
}

// 测试用:重置子页,避免模块级状态跨用例泄漏。
export function resetOwnerSubView(): void {
  setOwnerSubView(null)
}

function subscribeSubView(listener: () => void): () => void {
  subViewListeners.add(listener)
  return () => {
    subViewListeners.delete(listener)
  }
}

export function useOwnerSubView(): string | null {
  return useSyncExternalStore(subscribeSubView, getOwnerSubView, getOwnerSubView)
}
