export type StatusTone = 'draft' | 'submitted' | 'ai' | 'human' | 'approved' | 'rejected' | 'revising'

export type StatusMeta = {
  label: string
  tone: StatusTone
}

export const statusMeta: Record<string, StatusMeta> = {
  draft: { label: '草稿', tone: 'draft' },
  submitted: { label: '已提交', tone: 'submitted' },
  ai_reviewing: { label: 'AI 预审', tone: 'ai' },
  human_reviewing: { label: '人工审核', tone: 'human' },
  needs_arbitration: { label: '待仲裁', tone: 'revising' },
  approved: { label: '已通过', tone: 'approved' },
  rejected: { label: '已拒绝', tone: 'rejected' },
  revising: { label: '待修改', tone: 'revising' },
  queued: { label: '排队中', tone: 'submitted' },
  pending: { label: '待处理', tone: 'submitted' },
  running: { label: '运行中', tone: 'ai' },
  succeeded: { label: '已完成', tone: 'approved' },
  failed: { label: '失败', tone: 'rejected' },
  dead: { label: '已终止', tone: 'rejected' },
  published: { label: '已发布', tone: 'approved' },
  claimed: { label: '已领取', tone: 'human' },
  finished: { label: '已结束', tone: 'approved' },
}

export function statusLabel(status: string | null | undefined) {
  const key = normalizeStatus(status)
  return statusMeta[key]?.label ?? (status || '未知')
}

export function normalizeStatus(status: string | null | undefined) {
  return (status ?? '').trim().toLowerCase()
}
