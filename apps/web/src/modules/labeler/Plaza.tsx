import type { CSSProperties } from 'react'
import { useCallback, useEffect, useMemo, useState } from 'react'
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
  const [bundle, setBundle] = useState<TaskBundle | null>(null)
  const [answer, setAnswer] = useState<AnswerValue>({})
  const [errors, setErrors] = useState<ValidationError[]>([])
  const [loading, setLoading] = useState(false)

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

  const schema = useMemo(() => parseBundleSchema(bundle), [bundle])
  const payload = useMemo(() => parsePayload(bundle?.item?.payload), [bundle?.item?.payload])

  async function claim(taskId: number) {
    setLoading(true)
    try {
      const data = await apiPost<TaskBundle>(`/tasks/${taskId}/claim`, {})
      setBundle(data)
      setAnswer(parseAnswer(data.revision?.answer))
      setErrors([])
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
      const data = await apiPost<Submission>(`/tasks/${bundle.task.id}/items/${bundle.item.id}/draft`, { answer })
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
      Toast.success(`已提交，状态 ${data.status}`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '提交失败')
    }
  }

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
            <section style={panelStyle}>
              <div style={formHeaderStyle}>
                <h2 style={headingStyle}>{schema.ok ? schema.schema.title : '标注表单'}</h2>
                <span style={mutedStyle}>提交状态: {bundle.submission?.status || '未提交'}</span>
              </div>

              {schema.ok ? (
                <SchemaRenderer
                  schema={schema.schema}
                  payload={payload}
                  value={answer}
                  errors={errors}
                  runtime={{ taskId: bundle.task.id, itemId: bundle.item.id, submissionId: bundle.submission?.id }}
                  onChange={(next) => {
                    setAnswer(next)
                    if (errors.length > 0) {
                      setErrors(validateAnswer(schema.schema, next))
                    }
                  }}
                />
              ) : (
                <div role="alert" style={errorBannerStyle}>{schema.message}</div>
              )}

              <div style={actionRowStyle}>
                <Button disabled={!schema.ok} onClick={() => void saveDraft()}>保存草稿</Button>
                <Button disabled={!schema.ok} theme="solid" onClick={() => void submit()}>提交审核</Button>
              </div>
            </section>
          ) : (
            <section style={panelStyle}>
              <h2 style={headingStyle}>当前题目</h2>
              <p style={mutedStyle}>从左侧领取一条任务数据后开始标注。</p>
            </section>
          )}
        </main>
      </div>
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
  marginBottom: 'var(--space-lg)',
}

const errorBannerStyle: CSSProperties = {
  padding: 'var(--space-md)',
  border: '1px solid var(--color-danger, #b42318)',
  color: 'var(--color-danger, #b42318)',
  background: 'var(--color-bg)',
}

const actionRowStyle: CSSProperties = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-lg)',
}
