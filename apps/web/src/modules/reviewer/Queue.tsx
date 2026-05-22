import type { CSSProperties } from 'react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { SchemaRenderer, parseAnswer, parseTemplateSchema } from '../../renderer'
import type { AnswerValue, TemplateSchema } from '../../renderer/types'
import { apiGet, apiPost, type Submission, type TaskBundle } from '../../shared/api/client'
import { parsePayload } from '../../shared/components/payload'

type ReviewResponse = {
  submission_id: number
  status: string
}

type ParsedSchema =
  | { ok: true, schema: TemplateSchema }
  | { ok: false, message: string }

export default function ReviewerQueue() {
  const [submissions, setSubmissions] = useState<Submission[]>([])
  const [selected, setSelected] = useState<Submission | null>(null)
  const [detail, setDetail] = useState<TaskBundle | null>(null)
  const [reason, setReason] = useState('')
  const [loading, setLoading] = useState(false)

  const loadQueue = useCallback(async () => {
    try {
      const data = await apiGet<Submission[]>('/reviewer/submissions')
      setSubmissions(data)
      if (data.length === 0) {
        setSelected(null)
        setDetail(null)
      }
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载审核队列失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadQueue()
  }, [loadQueue])

  const schema = useMemo(() => parseBundleSchema(detail), [detail])
  const payload = useMemo(() => parsePayload(detail?.item?.payload), [detail?.item?.payload])
  const answer = useMemo<AnswerValue>(() => parseAnswer(detail?.revision?.answer), [detail?.revision?.answer])

  async function openSubmission(submission: Submission) {
    setSelected(submission)
    try {
      const data = await apiGet<TaskBundle>(`/reviewer/submissions/${submission.id}`)
      setDetail(data)
      setReason('')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载详情失败')
    }
  }

  async function review(verdict: 'approve' | 'reject' | 'revise') {
    if (!selected) {
      return
    }
    if ((verdict === 'reject' || verdict === 'revise') && reason.trim().length < 5) {
      Toast.error('打回/拒绝必须填写至少 5 字的理由')
      return
    }
    setLoading(true)
    try {
      const data = await apiPost<ReviewResponse>(`/submissions/${selected.id}/review`, { verdict, reason })
      Toast.success(`审核完成，状态 ${data.status}`)
      await loadQueue()
      setSelected(null)
      setDetail(null)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '审核失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div>
      <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)' }}>人工审核中心</h1>
      <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)' }}>审核队列 · 通过 / 拒绝 / 打回 · 审计流转</p>

      <div style={layoutStyle}>
        <section style={panelStyle}>
          <h2 style={headingStyle}>待审提交</h2>
          {submissions.map((submission) => (
            <button
              key={submission.id}
              onClick={() => void openSubmission(submission)}
              style={submission.id === selected?.id ? activeListButtonStyle : listButtonStyle}
            >
              <strong>Submission #{submission.id}</strong>
              <span>Task #{submission.taskId} · Item #{submission.itemId}</span>
              <span>{submission.status}</span>
            </button>
          ))}
          {submissions.length === 0 ? <p style={mutedStyle}>当前没有 human_reviewing 数据</p> : null}
        </section>

        <main style={{ display: 'grid', gap: 'var(--space-lg)' }}>
          {detail?.item ? (
            <>
              <section style={panelStyle}>
                <div style={formHeaderStyle}>
                  <h2 style={headingStyle}>{schema.ok ? schema.schema.title : '提交详情'}</h2>
                  <span style={mutedStyle}>Submission #{detail.submission?.id}</span>
                </div>
                {schema.ok ? (
                  <SchemaRenderer
                    schema={schema.schema}
                    payload={payload}
                    value={answer}
                    readOnly
                    runtime={{ taskId: detail.task.id, itemId: detail.item.id, submissionId: detail.submission?.id }}
                  />
                ) : (
                  <div role="alert" style={errorBannerStyle}>{schema.message}</div>
                )}
              </section>
              <section style={panelStyle}>
                <h2 style={headingStyle}>审核处理</h2>
                <p style={{ color: 'var(--color-text-secondary)', margin: 'var(--space-sm) 0 0' }}>
                  打回 / 拒绝 必须填写至少 5 字的具体理由,labeler 会在我的提交中看到。
                </p>
                <textarea
                  value={reason}
                  onChange={(event) => setReason(event.target.value)}
                  rows={4}
                  style={textareaStyle}
                  placeholder="例如:第 2 维度评分依据不足;corrected_answer JSON 缺少 reference 字段"
                />
                <div style={actionRowStyle}>
                  <Button disabled={!schema.ok} loading={loading} onClick={() => void review('revise')}>打回修改</Button>
                  <Button disabled={!schema.ok} loading={loading} onClick={() => void review('reject')}>拒绝</Button>
                  <Button disabled={!schema.ok} loading={loading} theme="solid" onClick={() => void review('approve')}>通过</Button>
                </div>
              </section>
            </>
          ) : (
            <section style={panelStyle}>
              <h2 style={headingStyle}>提交详情</h2>
              <p style={mutedStyle}>从左侧队列选择一条提交进行人工审核。</p>
            </section>
          )}
        </main>
      </div>
    </div>
  )
}

function parseBundleSchema(bundle: TaskBundle | null): ParsedSchema {
  if (!bundle?.template?.schemaJson) {
    return { ok: false, message: '当前提交缺少模板快照' }
  }
  const result = parseTemplateSchema(bundle.template.schemaJson)
  if (!result.ok) {
    return { ok: false, message: `${result.error.field}: ${result.error.message}` }
  }
  return { ok: true, schema: result.value }
}

const layoutStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '300px minmax(0, 1fr)',
  gap: 'var(--space-lg)',
  marginTop: 'var(--space-lg)',
}

const panelStyle: CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border)',
  padding: 'var(--space-lg)',
}

const headingStyle: CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h2)',
  margin: 0,
}

const mutedStyle: CSSProperties = {
  color: 'var(--color-text-secondary)',
}

const listButtonStyle: CSSProperties = {
  display: 'grid',
  gap: 4,
  width: '100%',
  marginTop: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  textAlign: 'left',
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
}

const activeListButtonStyle: CSSProperties = {
  ...listButtonStyle,
  borderColor: 'var(--color-accent)',
  background: 'var(--color-bg)',
}

const formHeaderStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  gap: 'var(--space-md)',
  alignItems: 'center',
  marginBottom: 'var(--space-lg)',
}

const errorBannerStyle: CSSProperties = {
  padding: 'var(--space-md)',
  border: '1px solid var(--color-danger, #b42318)',
  color: 'var(--color-danger, #b42318)',
  background: 'var(--color-bg)',
}

const textareaStyle: CSSProperties = {
  width: '100%',
  marginTop: 'var(--space-md)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  fontFamily: 'var(--font-body)',
  resize: 'vertical',
}

const actionRowStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-md)',
}
