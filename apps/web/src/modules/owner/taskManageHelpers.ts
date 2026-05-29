import type { Task, TaskDistribution } from '../../shared/api/client'

// 任务负责人后台的纯函数:标签/奖励/富文本 JSON 解析、统计卡计算、状态展示。
// 抽出来便于单测,且让 TaskForm / TaskManagePanel 保持聚焦。

export const DISTRIBUTION_LABELS: Record<TaskDistribution, string> = {
  first_come: '先到先得',
  assigned: '指派',
  quota: '配额抢单',
}

// 任务状态 → 中文展示标签。沿用后端 draft/published/paused/ended 四态。
export const TASK_STATUS_LABELS: Record<string, string> = {
  draft: '草稿',
  published: '发布中',
  paused: '已暂停',
  ended: '已结束',
}

export function taskStatusLabel(status: string): string {
  return TASK_STATUS_LABELS[status] ?? status
}

export function distributionLabel(distribution: string): string {
  return DISTRIBUTION_LABELS[distribution as TaskDistribution] ?? distribution
}

// 富文本说明用最简单的 {html:string} 形态存进 richDescription JSON 列。
export type RichDescription = { html: string }

export function parseRichDescriptionHtml(raw: string | null | undefined): string {
  if (!raw) return ''
  try {
    const parsed = JSON.parse(raw) as unknown
    if (parsed && typeof parsed === 'object' && typeof (parsed as RichDescription).html === 'string') {
      return (parsed as RichDescription).html
    }
    if (typeof parsed === 'string') return parsed
  } catch {
    return raw
  }
  return ''
}

export function richDescriptionPayload(html: string): RichDescription | null {
  const trimmed = html.trim()
  return trimmed ? { html: trimmed } : null
}

// tags 存为 JSON 字符串数组。解析时容错非数组/非字符串元素。
export function parseTags(raw: string | null | undefined): string[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.filter((tag): tag is string => typeof tag === 'string' && tag.trim() !== '')
  } catch {
    return []
  }
}

// 奖励规则用 {amount:number, unit:string} 的轻量结构,UI 上展示成「金额 + 单位」。
export type RewardConfig = { amount: number, unit: string }

export function parseRewardConfig(raw: string | null | undefined): RewardConfig {
  if (!raw) return { amount: 0, unit: '元/条' }
  try {
    const parsed = JSON.parse(raw) as unknown
    if (parsed && typeof parsed === 'object') {
      const record = parsed as Record<string, unknown>
      const amount = typeof record.amount === 'number' && Number.isFinite(record.amount) ? record.amount : 0
      const unit = typeof record.unit === 'string' && record.unit ? record.unit : '元/条'
      return { amount, unit }
    }
  } catch {
    return { amount: 0, unit: '元/条' }
  }
  return { amount: 0, unit: '元/条' }
}

export function rewardConfigPayload(amount: number, unit: string): RewardConfig | null {
  const cleanUnit = unit.trim()
  if (!Number.isFinite(amount) || amount <= 0) {
    return cleanUnit ? { amount: 0, unit: cleanUnit } : null
  }
  return { amount, unit: cleanUnit || '元/条' }
}

export function rewardSummary(raw: string | null | undefined): string {
  const reward = parseRewardConfig(raw)
  if (reward.amount <= 0) return '—'
  return `${reward.amount} ${reward.unit}`
}

// 截止时间 ISO 字符串 → datetime-local input 需要的 "YYYY-MM-DDTHH:mm" 本地格式。
export function isoToDatetimeLocal(iso: string | null | undefined): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

// datetime-local input 的本地值 → 后端要的 ISO 字符串。空值返回 null(显式清空截止时间)。
export function datetimeLocalToISO(value: string): string | null {
  const trimmed = value.trim()
  if (!trimmed) return null
  const date = new Date(trimmed)
  if (Number.isNaN(date.getTime())) return null
  return date.toISOString()
}

export function formatDeadline(iso: string | null | undefined): string {
  if (!iso) return '无截止'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '无截止'
  return date.toLocaleString()
}

export type TaskStat = {
  key: string
  label: string
  value: string
  tone?: 'primary' | 'success' | 'warning'
}

// 统计卡片完全从真实任务列表算出来,不造数据。
export function computeTaskStats(tasks: Task[]): TaskStat[] {
  const published = tasks.filter((task) => task.status === 'published').length
  const draft = tasks.filter((task) => task.status === 'draft').length
  const totalItems = tasks.reduce((sum, task) => sum + (task.totalItems || 0), 0)
  const finishedItems = tasks.reduce((sum, task) => sum + (task.finishedItems || 0), 0)
  const pending = Math.max(0, totalItems - finishedItems)
  return [
    { key: 'published', label: '发布中任务', value: String(published), tone: 'primary' },
    { key: 'draft', label: '草稿', value: String(draft) },
    { key: 'finished', label: '已完成题目', value: finishedItems.toLocaleString(), tone: 'success' },
    { key: 'pending', label: '待完成题目', value: pending.toLocaleString(), tone: 'warning' },
  ]
}

// 任务进度百分比;无题目时返回 null(展示 —)。
export function taskProgressPercent(task: Task): number | null {
  if (!task.totalItems || task.totalItems <= 0) return null
  return Math.min(100, Math.round((task.finishedItems / task.totalItems) * 100))
}

// 状态对应的状态机可用动作。draft 只能发布;published 可暂停/结束;paused 可恢复/结束。
export type TaskTransition = 'publish' | 'pause' | 'resume' | 'end'

export function availableTransitions(status: string): TaskTransition[] {
  switch (status) {
    case 'draft':
      return ['publish']
    case 'published':
      return ['pause', 'end']
    case 'paused':
      return ['resume', 'end']
    default:
      return []
  }
}

export const TRANSITION_LABELS: Record<TaskTransition, string> = {
  publish: '发布',
  pause: '暂停',
  resume: '恢复',
  end: '下线',
}
