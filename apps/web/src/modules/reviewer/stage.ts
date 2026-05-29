import type { ReviewStage } from '../../shared/api/client'

// 人工审核三级:初审 → 复审 → 终审。后端用 reviewStage/reviewLevel 暴露当前级别。
// 一次「通过」只推进一级,凑满 requiredLevels(默认 3)才会真正 approved。
export const STAGE_LABELS: Record<ReviewStage, string> = {
  first: '初审',
  second: '复审',
  final: '终审',
}

export type StageDisplay = {
  label: string
  level: number
  required: number
  progress: string
}

// 从 queue item / detail bundle 的扁平字段派生展示用的阶段信息。
// 字段缺失时退回初审 1/3,保证旧响应也能渲染。
export function resolveStage(input: {
  reviewStage?: ReviewStage
  reviewLevel?: number
  requiredLevels?: number
}): StageDisplay {
  const stage: ReviewStage = input.reviewStage ?? 'first'
  const required = input.requiredLevels && input.requiredLevels > 0 ? input.requiredLevels : 3
  const level = input.reviewLevel && input.reviewLevel > 0 ? input.reviewLevel : 1
  return {
    label: STAGE_LABELS[stage] ?? STAGE_LABELS.first,
    level,
    required,
    progress: `${level}/${required}`,
  }
}
