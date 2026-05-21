import type { ChangeEvent, CSSProperties } from 'react'
import { useCallback, useEffect, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { apiGet, apiPost, apiUpload, type Submission, type Task, type TaskBundle } from '../../shared/api/client'
import ShowItem from '../../shared/components/ShowItem'
import { parsePayload } from '../../shared/components/payload'

type UploadedFile = {
  id: number
  storageKey: string
  originalName: string
}

type LabelAnswer = {
  relevance_score: number
  accuracy_score: number
  format_score: number
  safety_score: number
  issue_tags: string[]
  summary: string
  comment: string
  revision_suggestion: string
  corrected_answer: string
  evidence_files: string[]
  ai_precheck: string
}

const emptyAnswer: LabelAnswer = {
  relevance_score: 0,
  accuracy_score: 0,
  format_score: 0,
  safety_score: 0,
  issue_tags: [],
  summary: '',
  comment: '',
  revision_suggestion: '',
  corrected_answer: '{\n  "corrected_answer": ""\n}',
  evidence_files: [],
  ai_precheck: '',
}

export default function LabelerPlaza() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [bundle, setBundle] = useState<TaskBundle | null>(null)
  const [answer, setAnswer] = useState<LabelAnswer>(emptyAnswer)
  const [loading, setLoading] = useState(false)
  const [aiLoading, setAILoading] = useState(false)
  const [uploading, setUploading] = useState(false)

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<Task[]>('/labeler/tasks')
      setTasks(data)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载任务失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTasks()
  }, [loadTasks])

  async function claim(taskId: number) {
    setLoading(true)
    try {
      const data = await apiPost<TaskBundle>(`/tasks/${taskId}/claim`, {})
      setBundle(data)
      setAnswer(prefillAnswer(data))
      Toast.success('已领取题目')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '领取失败')
    } finally {
      setLoading(false)
    }
  }

  async function saveDraft() {
    if (!bundle?.task || !bundle.item) {
      return
    }
    try {
      const data = await apiPost<Submission>(`/tasks/${bundle.task.id}/items/${bundle.item.id}/draft`, { answer: buildAnswer() })
      setBundle({ ...bundle, submission: data })
      Toast.success('草稿已保存')
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '保存失败')
    }
  }

  async function submit() {
    if (!bundle?.task || !bundle.item) {
      return
    }
    const validationError = validateAnswer(answer)
    if (validationError) {
      Toast.error(validationError)
      return
    }

    try {
      const data = await apiPost<Submission>(`/tasks/${bundle.task.id}/items/${bundle.item.id}/submit`, { answer: buildAnswer() })
      setBundle({ ...bundle, submission: data })
      Toast.success(`已提交，状态 ${data.status}`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '提交失败')
    }
  }

  async function runAI() {
    if (!bundle?.item) {
      return
    }
    setAILoading(true)
    try {
      const payload = parsePayload(bundle.item.payload)
      const data = await apiPost<{ text: string, provider: string }>('/llm/inline', {
        prompt: '请按 qa_quality 官方维度给当前标注做预审建议。',
        input: { payload, answer: buildAnswer() },
      })
      setAnswer((current) => ({ ...current, ai_precheck: data.text }))
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : 'AI 预审失败')
    } finally {
      setAILoading(false)
    }
  }

  async function uploadEvidence(event: ChangeEvent<HTMLInputElement>) {
    if (!bundle?.task) {
      return
    }
    const file = event.target.files?.[0]
    if (!file) {
      return
    }
    setUploading(true)
    try {
      const form = new FormData()
      form.append('task_id', String(bundle.task.id))
      form.append('file', file)
      const uploaded = await apiUpload<UploadedFile>('/uploads', form)
      setAnswer((current) => ({
        ...current,
        evidence_files: [...current.evidence_files, uploaded.storageKey],
      }))
      Toast.success(`已上传 ${uploaded.originalName}`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '上传失败')
    } finally {
      event.target.value = ''
      setUploading(false)
    }
  }

  function buildAnswer() {
    return {
      ...answer,
      corrected_answer: parseJSONOrText(answer.corrected_answer),
    }
  }

  const payload = parsePayload(bundle?.item?.payload)

  return (
    <div>
      <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)' }}>标注员工作台</h1>
      <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)' }}>任务广场 · 领取 · 作答 · 草稿 · 提交</p>

      <div style={layoutStyle}>
        <section style={panelStyle}>
          <h2 style={headingStyle}>任务广场</h2>
          {tasks.map((task) => (
            <div key={task.id} style={taskCardStyle}>
              <strong>{task.title}</strong>
              <span style={mutedStyle}>{task.finishedItems}/{task.totalItems} · {task.status}</span>
              <Button loading={loading} onClick={() => void claim(task.id)} style={{ marginTop: 'var(--space-sm)' }}>
                领取题目
              </Button>
            </div>
          ))}
          {tasks.length === 0 ? <p style={mutedStyle}>暂无可领取任务</p> : null}
        </section>

        <main style={{ display: 'grid', gap: 'var(--space-lg)' }}>
          {bundle?.item ? (
            <>
              <ShowItem payload={payload} />
              <section style={panelStyle}>
                <div style={formHeaderStyle}>
                  <h2 style={headingStyle}>标注表单</h2>
                  <span style={mutedStyle}>提交状态: {bundle.submission?.status || '未提交'}</span>
                </div>

                <ScoreRow label="相关性" value={answer.relevance_score} onChange={(value) => setAnswer({ ...answer, relevance_score: value })} />
                <ScoreRow label="准确性" value={answer.accuracy_score} onChange={(value) => setAnswer({ ...answer, accuracy_score: value })} />
                <ScoreRow label="格式合规" value={answer.format_score} onChange={(value) => setAnswer({ ...answer, format_score: value })} />
                <ScoreRow label="安全性" value={answer.safety_score} onChange={(value) => setAnswer({ ...answer, safety_score: value })} />

                <Field label="问题类型">
                  <div style={tagRowStyle}>
                    {['事实错误', '遗漏要点', '格式问题', '安全风险', '表达冗余', '无明显问题'].map((tag) => (
                      <label key={tag} style={checkboxStyle}>
                        <input
                          type="checkbox"
                          checked={answer.issue_tags.includes(tag)}
                          onChange={() => setAnswer(toggleTag(answer, tag))}
                        />
                        {tag}
                      </label>
                    ))}
                  </div>
                </Field>

                <Field label="一句话总评">
                  <input value={answer.summary} onChange={(event) => setAnswer({ ...answer, summary: event.target.value })} maxLength={80} style={inputStyle} />
                </Field>
                <Field label="详细评语">
                  <textarea value={answer.comment} onChange={(event) => setAnswer({ ...answer, comment: event.target.value })} rows={4} style={textareaStyle} />
                </Field>
                <Field label="修订建议">
                  <textarea value={answer.revision_suggestion} onChange={(event) => setAnswer({ ...answer, revision_suggestion: event.target.value })} rows={3} style={textareaStyle} />
                </Field>
                <Field label="修正后答案 JSON">
                  <textarea value={answer.corrected_answer} onChange={(event) => setAnswer({ ...answer, corrected_answer: event.target.value })} rows={5} style={monoTextareaStyle} />
                </Field>
                <Field label="证据素材">
                  <input type="file" disabled={uploading} onChange={(event) => void uploadEvidence(event)} />
                  {answer.evidence_files.length > 0 ? (
                    <pre style={smallPreStyle}>{JSON.stringify(answer.evidence_files, null, 2)}</pre>
                  ) : null}
                </Field>
                <Field label="AI 预评分">
                  <Button loading={aiLoading} onClick={() => void runAI()}>运行 Mock LLM</Button>
                  {answer.ai_precheck ? <pre style={smallPreStyle}>{answer.ai_precheck}</pre> : null}
                </Field>

                <div style={actionRowStyle}>
                  <Button onClick={() => void saveDraft()}>保存草稿</Button>
                  <Button theme="solid" onClick={() => void submit()}>提交审核</Button>
                </div>
              </section>
            </>
          ) : (
            <section style={panelStyle}>
              <h2 style={headingStyle}>当前题目</h2>
              <p style={mutedStyle}>从左侧领取一条官方 qa_quality 数据后开始标注。</p>
            </section>
          )}
        </main>
      </div>
    </div>
  )
}

function prefillAnswer(bundle: TaskBundle): LabelAnswer {
  if (!bundle.revision?.answer) {
    return emptyAnswer
  }
  try {
    const parsed = JSON.parse(bundle.revision.answer) as Partial<LabelAnswer> & { corrected_answer?: unknown }
    return {
      ...emptyAnswer,
      ...parsed,
      corrected_answer: typeof parsed.corrected_answer === 'string'
        ? parsed.corrected_answer
        : JSON.stringify(parsed.corrected_answer ?? { corrected_answer: '' }, null, 2),
    }
  } catch {
    return emptyAnswer
  }
}

function validateAnswer(value: LabelAnswer) {
  if (!value.relevance_score || !value.accuracy_score || !value.format_score || !value.safety_score) {
    return '四个评分维度都必须填写'
  }
  if (!value.summary.trim()) {
    return '一句话总评必填'
  }
  if (value.comment.trim().length < 10) {
    return '详细评语至少 10 个字'
  }
  return ''
}

function parseJSONOrText(value: string) {
  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

function toggleTag(answer: LabelAnswer, tag: string): LabelAnswer {
  const exists = answer.issue_tags.includes(tag)
  return {
    ...answer,
    issue_tags: exists ? answer.issue_tags.filter((item) => item !== tag) : [...answer.issue_tags, tag],
  }
}

function Field({ label, children }: { label: string, children: React.ReactNode }) {
  return (
    <label style={fieldStyle}>
      <span style={fieldLabelStyle}>{label}</span>
      {children}
    </label>
  )
}

function ScoreRow({ label, value, onChange }: { label: string, value: number, onChange: (value: number) => void }) {
  return (
    <Field label={label}>
      <div style={scoreRowStyle}>
        {[1, 2, 3, 4, 5].map((score) => (
          <label key={score} style={radioStyle}>
            <input type="radio" checked={value === score} onChange={() => onChange(score)} />
            {score}
          </label>
        ))}
      </div>
    </Field>
  )
}

const layoutStyle: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '280px minmax(0, 1fr)',
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

const taskCardStyle: CSSProperties = {
  display: 'grid',
  gap: 4,
  marginTop: 'var(--space-md)',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border-light)',
}

const formHeaderStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'space-between',
  gap: 'var(--space-md)',
  alignItems: 'center',
}

const fieldStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  marginTop: 'var(--space-md)',
}

const fieldLabelStyle: CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontWeight: 600,
}

const inputStyle: CSSProperties = {
  minHeight: 36,
  padding: '0 var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  fontFamily: 'var(--font-body)',
}

const textareaStyle: CSSProperties = {
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  fontFamily: 'var(--font-body)',
  resize: 'vertical',
}

const monoTextareaStyle: CSSProperties = {
  ...textareaStyle,
  fontFamily: 'var(--font-mono)',
}

const scoreRowStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  flexWrap: 'wrap',
}

const radioStyle: CSSProperties = {
  display: 'inline-flex',
  gap: 4,
  alignItems: 'center',
  padding: '4px 8px',
  border: '1px solid var(--color-border-light)',
}

const tagRowStyle: CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  flexWrap: 'wrap',
}

const checkboxStyle: CSSProperties = {
  display: 'inline-flex',
  gap: 4,
  alignItems: 'center',
}

const smallPreStyle: CSSProperties = {
  marginTop: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  background: 'var(--color-bg)',
  border: '1px solid var(--color-border-light)',
  whiteSpace: 'pre-wrap',
}

const actionRowStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-lg)',
}
