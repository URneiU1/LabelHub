import type { AIReviewDetail, Submission } from '../../shared/api/client'
import { Icon } from '../../shared/components/Icon'
import StatusBadge from '../../shared/components/StatusBadge'
import { formatScore } from './format'

// AI 预审面板:复刻 ui-demo AiReview 的视觉(维度 ScoreBar + 评语 + 状态),
// 数据全部来自真实 aiReview / submission,不造假。
type AIVerdictPanelProps = {
  aiReview?: AIReviewDetail | null
  submission?: Submission | null
}

type DimensionRow = { label: string, value: number }

const PASS_THRESHOLD = 80

export default function AIVerdictPanel({ aiReview, submission }: AIVerdictPanelProps) {
  if (aiReview) {
    const score = formatScore(aiReview.overallScore)
    const verdict = aiReview.verdict ?? submission?.aiVerdict ?? 'unknown'
    const displayVerdict = verdictLabel(verdict)
    const dimensions = normalizeDimensions(aiReview.dimensions)
    const tone = verdictTone(verdict)
    return (
      <section className="ai-section" aria-label="AI 预审结论">
        <div className="ai-section__head">
          <span className="ai-section__title lh-icon-text"><Icon name="sparkle" size={14} />AI 预审结论</span>
          <span className="ai-section__aside">
            <span className="lh-tag lh-tag--purple">{aiReview.prompt ? `规则：v${aiReview.prompt.version} · ${aiReview.prompt.model}` : `规则：v${aiReview.promptVersion}`}</span>
          </span>
        </div>
        <div className="hr-rerun__scores">AI 判定：{displayVerdict} · {score}</div>
        <div style={{ color: verdictColor(verdict), fontWeight: 700, fontSize: 13 }}>判定：{displayVerdict}</div>
        <div style={{ fontWeight: 700, fontSize: 13, marginBottom: 10 }}>综合分：{score}</div>
        {dimensions.length > 0 ? (
          <div style={{ marginBottom: 12 }}>
            {dimensions.map((row) => <ScoreBar key={row.label} label={row.label} value={row.value} />)}
          </div>
        ) : null}
        {aiReview.reason ? (
          <div className="ai-verdict" style={tone === 'success' ? successVerdictStyle : undefined}>
            <div className="ai-verdict__head">
              AI 评语 <span className={verdictPillClass(tone)}>{verdictLabel(verdict)}</span>{' '}
              <span className="lh-muted">阈值：综合 &lt; {PASS_THRESHOLD} 即打回</span>
            </div>
            {aiReview.reason}
          </div>
        ) : null}
        <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', alignItems: 'center' }}>
          <StatusBadge status={aiReview.status} />
          <span className="lh-muted">模型用量：{aiReview.tokensInput + aiReview.tokensOutput} Token</span>
          <span className="lh-muted">耗时：{aiReview.latencyMs}ms</span>
        </div>
      </section>
    )
  }

  if (submission?.aiVerdict) {
    const score = formatScore(submission.aiScore)
    const displayVerdict = verdictLabel(submission.aiVerdict)
    return (
      <section className="ai-section" aria-label="AI 预审结论">
        <div className="ai-section__head">
          <span className="ai-section__title lh-icon-text"><Icon name="sparkle" size={14} />AI 预审结论</span>
        </div>
        <div className="hr-rerun__scores">AI 判定：{displayVerdict} · {score}</div>
        <div style={{ color: verdictColor(submission.aiVerdict), fontWeight: 700, fontSize: 13 }}>判定：{displayVerdict}</div>
        <div style={{ fontWeight: 700, fontSize: 13 }}>综合分：{score}</div>
      </section>
    )
  }

  return (
    <section className="ai-section" aria-label="AI 预审结论">
      <div className="ai-section__head">
        <span className="ai-section__title lh-icon-text"><Icon name="sparkle" size={14} />AI 预审结论</span>
      </div>
      <div className="hr-rerun__scores">AI 未预审</div>
      <p className="lh-muted">暂无 AI 预审结果</p>
    </section>
  )
}

function ScoreBar({ label, value }: DimensionRow) {
  const color = scoreColor(label, value)
  return (
    <div className="ai-score-row">
      <span className="ai-score-row__label">{label}</span>
      <div className="ai-score-row__bar">
        <div className="ai-score-row__fill" style={{ width: `${clampScore(value)}%`, background: color }} />
      </div>
      <span className="ai-score-row__num" style={{ color }}>{value}</span>
    </div>
  )
}

const successVerdictStyle = { background: 'var(--lh-success-soft)', borderColor: 'transparent' } as const

function verdictTone(verdict: string): 'success' | 'warning' | 'purple' | 'danger' {
  if (verdict === 'pass') return 'success'
  if (verdict === 'manual') return 'purple'
  if (verdict === 'reject') return 'warning'
  return 'danger'
}

function verdictLabel(verdict: string) {
  if (verdict === 'pass') return '建议通过'
  if (verdict === 'manual') return '转人工'
  if (verdict === 'uncertain') return '需人工复核'
  if (verdict === 'reject') return '建议打回'
  return verdict
}

function verdictColor(verdict: string) {
  if (verdict === 'pass') return 'var(--lh-success)'
  if (verdict === 'reject') return 'var(--lh-warning)'
  return 'var(--lh-danger)'
}

function verdictPillClass(tone: 'success' | 'warning' | 'purple' | 'danger') {
  return tone === 'warning'
    ? 'ai-verdict__head-pill'
    : `ai-verdict__head-pill ai-verdict__head-pill--${tone}`
}

// 安全维度低分用红,其余低分用橙,达标用绿,贴合 ui-demo 的配色语义。
function scoreColor(label: string, value: number) {
  if (value >= PASS_THRESHOLD) return 'var(--lh-success)'
  if (label.includes('安全')) return 'var(--lh-danger)'
  return 'var(--lh-warning)'
}

function clampScore(value: number) {
  return Math.max(0, Math.min(100, value))
}

function normalizeDimensions(raw: unknown): DimensionRow[] {
  if (!Array.isArray(raw)) {
    return []
  }
  return raw.flatMap((item) => {
    if (!item || typeof item !== 'object') {
      return []
    }
    const record = item as Record<string, unknown>
    const label = typeof record.name === 'string' ? record.name : typeof record.label === 'string' ? record.label : ''
    const value = typeof record.score === 'number' ? record.score : typeof record.value === 'number' ? record.value : NaN
    if (!label || !Number.isFinite(value)) {
      return []
    }
    return [{ label, value }]
  })
}
