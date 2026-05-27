import type { CSSProperties } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { SchemaRenderer, parseAnswer, parseTemplateSchema } from '../../renderer'
import type { AnswerValue, TemplateSchema, ValidationError } from '../../renderer/types'
import { validateAnswer } from '../../renderer/validator'
import { apiGet, apiPost, type Submission, type Task, type TaskBundle } from '../../shared/api/client'
import { parsePayload } from '../../shared/components/payload'

type ParsedSchema =
  | { ok: true, schema: TemplateSchema }
  | { ok: false, message: string }

export default function LabelerPlaza() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [mySubmissions, setMySubmissions] = useState<Submission[]>([])
  const [bundle, setBundle] = useState<TaskBundle | null>(null)
  const [answer, setAnswer] = useState<AnswerValue>({})
  const [errors, setErrors] = useState<ValidationError[]>([])
  const [loading, setLoading] = useState(false)
  const [autoSaveState, setAutoSaveState] = useState<'idle' | 'saving' | 'saved' | 'failed'>('idle')
  const lastSavedDraftKey = useRef('')
  const autoSaveSeq = useRef(0)

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<Task[]>('/labeler/tasks')
      setTasks(data)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载任务失败')
    }
  }, [])

  const loadMySubmissions = useCallback(async () => {
    try {
      const data = await apiGet<Submission[]>('/me/submissions')
      setMySubmissions(data)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载我的提交失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTasks()
    void loadMySubmissions()
  }, [loadMySubmissions, loadTasks])

  const schema = useMemo(() => parseBundleSchema(bundle), [bundle])
  const payload = useMemo(() => parsePayload(bundle?.item?.payload), [bundle?.item?.payload])

  async function claim(taskId: number) {
    setLoading(true)
    try {
      const data = await apiPost<TaskBundle>(`/tasks/${taskId}/claim`, {})
      setBundle(data)
      const nextAnswer = parseAnswer(data.revision?.answer)
      autoSaveSeq.current += 1
      lastSavedDraftKey.current = answerDraftKey(nextAnswer)
      setAnswer(nextAnswer)
      setErrors([])
      setAutoSaveState('idle')
      Toast.success('已领取题目')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '领取失败')
    } finally {
      setLoading(false)
    }
  }

  async function openSubmission(submission: Submission) {
    setLoading(true)
    try {
      const data = await apiGet<TaskBundle>(`/tasks/${submission.taskId}/items/${submission.itemId}`)
      setBundle(data)
      const nextAnswer = parseAnswer(data.revision?.answer)
      autoSaveSeq.current += 1
      lastSavedDraftKey.current = answerDraftKey(nextAnswer)
      setAnswer(nextAnswer)
      setErrors([])
      setAutoSaveState('idle')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载待修改任务失败')
    } finally {
      setLoading(false)
    }
  }

  async function saveDraft() {
    if (!bundle?.task || !bundle.item) {
      return
    }
    try {
      autoSaveSeq.current += 1
      const data = await apiPost<Submission>(`/tasks/${bundle.task.id}/items/${bundle.item.id}/draft`, { answer })
      setBundle({ ...bundle, submission: data })
      lastSavedDraftKey.current = answerDraftKey(answer)
      setAutoSaveState('saved')
      Toast.success('草稿已保存')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '保存失败')
    }
  }

  async function submit() {
    if (!bundle?.task || !bundle.item) {
      return
    }
    if (!schema.ok) {
      Toast.error(schema.message)
      return
    }
    const validationErrors = validateAnswer(schema.schema, answer)
    setErrors(validationErrors)
    if (validationErrors.length > 0) {
      Toast.error(validationErrors[0].message)
      return
    }

    try {
      const data = await apiPost<Submission>(`/tasks/${bundle.task.id}/items/${bundle.item.id}/submit`, { answer })
      setBundle({ ...bundle, submission: data })
      autoSaveSeq.current += 1
      lastSavedDraftKey.current = answerDraftKey(answer)
      setAutoSaveState('idle')
      void loadMySubmissions()
      Toast.success(`已提交，状态 ${data.status}`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '提交失败')
    }
  }

  const answerKey = useMemo(() => answerDraftKey(answer), [answer])

  useEffect(() => {
    if (!bundle?.task || !bundle.item || !schema.ok || answerKey === lastSavedDraftKey.current) {
      return
    }
    const taskId = bundle.task.id
    const itemId = bundle.item.id
    const timer = window.setTimeout(() => {
      // 在触发时捕获当前活动序号(已由最近一次 onChange / 手动保存推进),且不在定时器里改写它;
      // 任何更晚的改动或保存都会推进序号,使本次 autosave 的回调因序号不等而作废,
      // 杜绝旧闭包里的 answer 覆盖更新的 lastSavedDraftKey。
      const requestSeq = autoSaveSeq.current
      setAutoSaveState('saving')
      void apiPost<Submission>(`/tasks/${taskId}/items/${itemId}/draft`, { answer })
        .then((submission) => {
          if (autoSaveSeq.current !== requestSeq) {
            return
          }
          lastSavedDraftKey.current = answerKey
          setBundle((current) => current && current.task.id === taskId && current.item?.id === itemId ? { ...current, submission } : current)
          setAutoSaveState('saved')
        })
        .catch(() => {
          if (autoSaveSeq.current === requestSeq) {
            setAutoSaveState('failed')
          }
        })
    }, 3000)
    return () => window.clearTimeout(timer)
  }, [answer, answerKey, bundle?.item, bundle?.task, schema])

  return (
    <div>
      <div style={{ marginBottom: 'var(--space-xl)' }}>
        <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)', margin: 0, fontWeight: 700 }}>标注工作台</h1>
        <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)', marginTop: 'var(--space-xs)' }}>任务领取 · 在线作答 · AI 辅助 · 结果提交</p>
      </div>

      <div style={layoutStyle}>
        <section style={panelStyle}>
          <div style={{ borderBottom: '1px solid var(--color-border-light)', paddingBottom: 'var(--space-sm)', marginBottom: 'var(--space-md)' }}>
            <h2 style={headingStyle}>任务广场</h2>
          </div>
          <div style={{ display: 'grid', gap: 'var(--space-md)' }}>
            {tasks.map((task) => (
              <div key={task.id} style={taskCardStyle}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <strong style={{ fontSize: 'var(--text-base)' }}>{task.title}</strong>
                </div>
                <div style={{ display: 'flex', gap: 'var(--space-sm)', marginTop: 4 }}>
                  <span style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }}>完成: {task.finishedItems}/{task.totalItems}</span>
                  <span style={{ fontSize: 'var(--text-sm)', color: 'var(--color-accent)', fontWeight: 600 }}>{task.status.toUpperCase()}</span>
                </div>
                <ProgressBar value={task.finishedItems} total={task.totalItems} />
                <Button aria-label="领取题目" loading={loading} onClick={() => void claim(task.id)} theme="solid" style={{ marginTop: 'var(--space-md)' }}>
                  领取新题目
                </Button>
              </div>
            ))}
            {tasks.length === 0 ? (
              <div style={{ padding: 'var(--space-xl)', textAlign: 'center', color: 'var(--color-text-muted)', background: 'var(--color-bg)', borderRadius: 'var(--radius-md)' }}>
                暂无可领取任务
              </div>
            ) : null}
          </div>
          {mySubmissions.some((submission) => submission.status === 'revising') ? (
            <div style={revisionListStyle}>
              <h3 style={revisionHeadingStyle}>待修改</h3>
              {mySubmissions.filter((submission) => submission.status === 'revising').map((submission) => (
                <button
                  key={submission.id}
                  aria-label={`修改 Submission #${submission.id}`}
                  onClick={() => void openSubmission(submission)}
                  style={revisionButtonStyle}
                >
                  <strong>Submission #{submission.id}</strong>
                  <span>Task #{submission.taskId} · Item #{submission.itemId}</span>
                </button>
              ))}
            </div>
          ) : null}
        </section>

        <main style={{ display: 'grid', gap: 'var(--space-lg)', alignContent: 'start' }}>
          {bundle?.item ? (
            <section style={{ ...panelStyle, minHeight: 600 }}>
              <div style={formHeaderStyle}>
                <div>
                  <h2 style={{ ...headingStyle, fontSize: 'var(--text-h1)' }}>{schema.ok ? schema.schema.title : '标注表单'}</h2>
                  <div style={{ display: 'flex', gap: 'var(--space-md)', marginTop: 'var(--space-xs)' }}>
                    <span style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)' }}>任务 ID: {bundle.task.id}</span>
                    <span style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)' }}>题目 ID: {bundle.item.id}</span>
                  </div>
                </div>
                <div style={{ textAlign: 'right' }}>
                  <div style={{ padding: '4px 12px', borderRadius: 12, background: bundle.submission ? '#e8f5e9' : '#fff3e0', color: bundle.submission ? '#2e7d32' : '#ef6c00', fontSize: 12, fontWeight: 'bold' }}>
                    {bundle.submission?.status.toUpperCase() || 'NEW'}
                  </div>
                </div>
              </div>

              <div style={{ borderTop: '1px solid var(--color-border-light)', paddingTop: 'var(--space-xl)', marginTop: 'var(--space-md)' }}>
                {bundle.latestHumanReview?.reason ? (
                  <div style={revisionReasonStyle}>
                    <strong>上一轮打回意见</strong>
                    <div>{bundle.latestHumanReview.reason}</div>
                  </div>
                ) : null}
                {schema.ok ? (
                  <SchemaRenderer
                    schema={schema.schema}
                    payload={payload}
                    value={answer}
                    errors={errors}
                    runtime={{ taskId: bundle.task.id, itemId: bundle.item.id, submissionId: bundle.submission?.id }}
                    onChange={(next) => {
                      autoSaveSeq.current += 1
                      setAnswer(next)
                      if (answerDraftKey(next) !== lastSavedDraftKey.current) {
                        setAutoSaveState('idle')
                      }
                      if (errors.length > 0) {
                        setErrors(validateAnswer(schema.schema, next))
                      }
                    }}
                  />
                ) : (
                  <div role="alert" style={errorBannerStyle}>{schema.message}</div>
                )}
              </div>

              <div style={{ ...actionRowStyle, borderTop: '1px solid var(--color-border-light)', paddingTop: 'var(--space-lg)', marginTop: 'var(--space-2xl)' }}>
                <Button disabled={!schema.ok} onClick={() => void saveDraft()} theme="light" style={{ width: 120 }}>保存草稿</Button>
                <span style={autoSaveTextStyle}>{autoSaveText(autoSaveState)}</span>
                <Button disabled={!schema.ok} theme="solid" onClick={() => void submit()} style={{ width: 120 }}>提交审核</Button>
              </div>
            </section>
          ) : (
            <section style={{ ...panelStyle, border: '1px dashed var(--color-border-light)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: 400 }}>
              <SchematicEmptyState />
              <h2 style={{ ...headingStyle, color: 'var(--color-text-muted)' }}>准备开始标注</h2>
              <p style={{ ...mutedStyle, marginTop: 'var(--space-sm)' }}>请在左侧任务广场选择并领取一个任务开始工作。</p>
            </section>
          )}
        </main>
      </div>
    </div>
  )
}

function ProgressBar({ value, total }: { value: number, total: number }) {
  const width = total > 0 ? Math.min(100, Math.round((value / total) * 100)) : 0
  return (
    <div style={progressTrackStyle} aria-hidden="true">
      <div style={{ ...progressFillStyle, width: `${width}%` }} />
    </div>
  )
}

function SchematicEmptyState() {
  return (
    <div style={emptyDiagramStyle} aria-hidden="true">
      <span style={emptyNodeStyle} />
      <span style={emptyLineStyle} />
      <span style={{ ...emptyNodeStyle, borderColor: 'var(--color-accent)' }} />
    </div>
  )
}

function parseBundleSchema(bundle: TaskBundle | null): ParsedSchema {
  if (!bundle?.template?.schemaJson) {
    return { ok: false, message: '当前任务未配置标注模板' }
  }
  const result = parseTemplateSchema(bundle.template.schemaJson)
  if (!result.ok) {
    return { ok: false, message: `${result.error.field}: ${result.error.message}` }
  }
  return { ok: true, schema: result.value }
}

function answerDraftKey(answer: AnswerValue) {
  return JSON.stringify(answer)
}

function autoSaveText(state: 'idle' | 'saving' | 'saved' | 'failed') {
  switch (state) {
    case 'saving':
      return '自动保存中...'
    case 'saved':
      return '已自动保存'
    case 'failed':
      return '自动保存失败'
    default:
      return '3s 自动保存'
  }
}

const layoutStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '320px minmax(0, 1fr)',
  gap: 'var(--space-xl)',
  alignItems: 'start',
}

const panelStyle: CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-lg)',
  padding: 'var(--space-xl)',
  boxShadow: 'var(--shadow-md)',
}

const headingStyle: CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h2)',
  margin: 0,
  fontWeight: 600,
  color: 'var(--color-text)',
}

const mutedStyle: CSSProperties = {
  color: 'var(--color-text-muted)',
  fontSize: 'var(--text-base)',
}

const taskCardStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  padding: 'var(--space-lg)',
  background: 'var(--color-bg)',
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  transition: 'transform var(--duration-fast)',
  borderLeft: '3px solid var(--color-rail)',
}

const revisionListStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-xl)',
  paddingTop: 'var(--space-lg)',
  borderTop: '1px solid var(--color-border-light)',
}

const revisionHeadingStyle: CSSProperties = {
  ...headingStyle,
  fontSize: 'var(--text-base)',
}

const revisionButtonStyle: CSSProperties = {
  display: 'grid',
  gap: 4,
  width: '100%',
  padding: 'var(--space-md)',
  textAlign: 'left',
  border: '1px solid var(--color-warning-soft)',
  borderRadius: 'var(--radius-md)',
  background: '#fff8e1',
  color: 'var(--color-text)',
  cursor: 'pointer',
}

const revisionReasonStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  marginBottom: 'var(--space-lg)',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-warning-soft)',
  borderRadius: 'var(--radius-md)',
  background: '#fff8e1',
  color: 'var(--color-text)',
}

const progressTrackStyle: CSSProperties = {
  height: 4,
  marginTop: 'var(--space-xs)',
  background: 'var(--color-border-light)',
  borderRadius: 99,
  overflow: 'hidden',
}

const progressFillStyle: CSSProperties = {
  height: '100%',
  background: 'var(--color-accent)',
}

const formHeaderStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  gap: 'var(--space-md)',
  alignItems: 'flex-start',
}

const errorBannerStyle: CSSProperties = {
  padding: 'var(--space-lg)',
  borderRadius: 'var(--radius-md)',
  border: '1px solid var(--color-danger)',
  color: 'var(--color-danger)',
  background: '#fff1f0',
}

const actionRowStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: 'var(--space-md)',
}

const autoSaveTextStyle: CSSProperties = {
  alignSelf: 'center',
  marginRight: 'auto',
  color: 'var(--color-text-muted)',
  fontSize: 'var(--text-sm)',
}

const emptyDiagramStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '56px 44px 56px',
  alignItems: 'center',
  justifyItems: 'center',
  marginBottom: 'var(--space-md)',
}

const emptyNodeStyle: CSSProperties = {
  width: 52,
  height: 32,
  border: '1px solid var(--color-node-border)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-node-bg)',
  boxShadow: 'var(--shadow-sm)',
}

const emptyLineStyle: CSSProperties = {
  width: 44,
  height: 1,
  background: 'var(--color-node-border)',
}
