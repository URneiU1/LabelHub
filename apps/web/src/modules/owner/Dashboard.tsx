import { useCallback, useEffect, useState } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { apiGet, apiPost, type Task } from '../../shared/api/client'

type TaskListResponse = Task[]
type ExportResponse = {
  task: Task
  rows: Array<Record<string, unknown>>
}
type AIPromptConfig = {
  id: number
  version: number
  promptTemplate: string
  dimensions: string
  passThreshold: number
  uncertainMin: number
  model: string
}
type AIPromptsResponse = {
  prompts: AIPromptConfig[]
  activePromptId: number | null
  aiReviewEnabled: boolean
}
type AIDryRunResult = {
  provider: string
  result: {
    verdict: string
    overall_score: number
    dimensions: Array<{ name: string, score: number, reason: string }>
    reason: string
  }
}

export default function OwnerDashboard() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [selected, setSelected] = useState<Task | null>(null)
  const [exportRows, setExportRows] = useState<Array<Record<string, unknown>>>([])
  const [prompts, setPrompts] = useState<AIPromptConfig[]>([])
  const [activePromptId, setActivePromptId] = useState<number | null>(null)
  const [promptTemplate, setPromptTemplate] = useState('请根据 payload 和 answer 完成结构化预审。')
  const [dimensionsText, setDimensionsText] = useState('相关性\n准确性\n格式合规')
  const [passThreshold, setPassThreshold] = useState('80')
  const [uncertainMin, setUncertainMin] = useState('60')
  const [model, setModel] = useState('mock-model')
  const [samplePayload, setSamplePayload] = useState('{"prompt":"示例题目"}')
  const [sampleAnswer, setSampleAnswer] = useState('{"summary":"示例答案"}')
  const [dryRun, setDryRun] = useState<AIDryRunResult | null>(null)
  const [promptError, setPromptError] = useState('')
  const [savingPrompt, setSavingPrompt] = useState(false)
  const [runningDryRun, setRunningDryRun] = useState(false)

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<TaskListResponse>('/tasks')
      setTasks(data)
      setSelected(data[0] ?? null)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载任务失败')
    }
  }, [])

  const fillPromptForm = useCallback((prompt: AIPromptConfig) => {
    setPromptTemplate(prompt.promptTemplate)
    setDimensionsText(dimensionsToText(prompt.dimensions))
    setPassThreshold(String(prompt.passThreshold))
    setUncertainMin(String(prompt.uncertainMin))
    setModel(prompt.model)
  }, [])

  const loadPrompts = useCallback(async (taskId: number) => {
    setPrompts([])
    setActivePromptId(null)
    setDryRun(null)
    setPromptError('')
    try {
      const data = await apiGet<AIPromptsResponse>(`/tasks/${taskId}/ai-prompts`)
      setPrompts(data.prompts)
      setActivePromptId(data.activePromptId)
      setDryRun(null)
      setPromptError('')
      const latest = data.prompts[0]
      if (latest) {
        fillPromptForm(latest)
      }
    } catch (error) {
      setPrompts([])
      setActivePromptId(null)
      setDryRun(null)
      setPromptError(error instanceof Error ? error.message : '加载 AI Prompt 失败')
    }
  }, [fillPromptForm])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTasks()
  }, [loadTasks])

  useEffect(() => {
    if (selected) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      void loadPrompts(selected.id)
    }
  }, [loadPrompts, selected])

  async function exportJSON(taskId: number) {
    try {
      const data = await apiGet<ExportResponse>(`/tasks/${taskId}/export/json`)
      setExportRows(data.rows)
      Toast.success(`导出 ${data.rows.length} 条 approved 数据`)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '导出失败')
    }
  }

  async function savePrompt() {
    if (!selected) return
    setSavingPrompt(true)
    setPromptError('')
    try {
      const data = await apiPost<{ prompt: AIPromptConfig, activePromptId: number }>(`/tasks/${selected.id}/ai-prompts`, {
        prompt_template: promptTemplate,
        dimensions: dimensionsText.split('\n').map((name) => ({ name: name.trim() })).filter((dimension) => dimension.name),
        pass_threshold: Number(passThreshold),
        uncertain_min: Number(uncertainMin),
        model,
      })
      setPrompts((current) => [data.prompt, ...current])
      setActivePromptId(data.activePromptId)
      fillPromptForm(data.prompt)
      Toast.success('AI Prompt 已保存')
    } catch (error) {
      const message = error instanceof Error ? error.message : '保存 AI Prompt 失败'
      setPromptError(message)
      Toast.error(message)
    } finally {
      setSavingPrompt(false)
    }
  }

  async function runDryRun() {
    if (!selected || !activePromptId) return
    setRunningDryRun(true)
    setPromptError('')
    try {
      const data = await apiPost<AIDryRunResult>(`/tasks/${selected.id}/ai-prompts/${activePromptId}/dry-run`, {
        payload: JSON.parse(samplePayload) as Record<string, unknown>,
        answer: JSON.parse(sampleAnswer) as Record<string, unknown>,
      })
      setDryRun(data)
      Toast.success('dry-run 完成')
    } catch (error) {
      const message = error instanceof Error ? error.message : 'dry-run 失败'
      setPromptError(message)
      Toast.error(message)
    } finally {
      setRunningDryRun(false)
    }
  }

  return (
    <div>
      <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)' }}>Owner 任务负责人</h1>
      <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)' }}>任务管理 · baseline · 数据导出</p>

      <div style={{ display: 'grid', gridTemplateColumns: '280px minmax(0, 1fr)', gap: 'var(--space-lg)', marginTop: 'var(--space-lg)' }}>
        <section style={panelStyle}>
          <h2 style={headingStyle}>任务</h2>
          {tasks.map((task) => (
            <button key={task.id} onClick={() => setSelected(task)} style={task.id === selected?.id ? activeListButtonStyle : listButtonStyle}>
              <strong>{task.title}</strong>
              <span>{task.finishedItems}/{task.totalItems} · {task.status}</span>
            </button>
          ))}
        </section>

        <section style={panelStyle}>
          {selected ? (
            <>
              <h2 style={headingStyle}>{selected.title}</h2>
              <p style={{ color: 'var(--color-text-secondary)' }}>
                官方 qa_quality 主线任务。AI 预审在 Sprint 1 关闭，提交后直接进入人工审核。
              </p>
              <div style={{ marginTop: 'var(--space-md)', padding: 'var(--space-md)', border: '1px solid var(--color-border-light)', maxHeight: 260, overflow: 'auto', whiteSpace: 'pre-wrap' }}>
                {selected.baselineDescription || '暂无 baseline'}
              </div>
              <section style={aiPromptSectionStyle}>
                <h3 style={subHeadingStyle}>AI Prompt</h3>
                {promptError ? <div role="alert" style={alertStyle}>{promptError}</div> : null}
                <div style={formGridStyle}>
                  <label style={fieldStyle}>
                    prompt_template
                    <textarea aria-label="prompt_template" value={promptTemplate} onChange={(event) => setPromptTemplate(event.target.value)} style={textareaStyle} />
                  </label>
                  <label style={fieldStyle}>
                    dimensions
                    <textarea aria-label="dimensions" value={dimensionsText} onChange={(event) => setDimensionsText(event.target.value)} style={textareaStyle} />
                  </label>
                  <label style={fieldStyle}>
                    pass_threshold
                    <input aria-label="pass_threshold" type="number" value={passThreshold} onChange={(event) => setPassThreshold(event.target.value)} style={inputStyle} />
                  </label>
                  <label style={fieldStyle}>
                    uncertain_min
                    <input aria-label="uncertain_min" type="number" value={uncertainMin} onChange={(event) => setUncertainMin(event.target.value)} style={inputStyle} />
                  </label>
                  <label style={fieldStyle}>
                    model
                    <input aria-label="model" value={model} onChange={(event) => setModel(event.target.value)} style={inputStyle} />
                  </label>
                </div>
                <Button loading={savingPrompt} theme="solid" onClick={() => void savePrompt()} style={{ marginTop: 'var(--space-md)' }}>
                  保存 AI Prompt
                </Button>
                <div style={dryRunPanelStyle}>
                  <div style={dryRunGridStyle}>
                    <label style={fieldStyle}>
                      sample_payload
                      <textarea aria-label="sample_payload" value={samplePayload} onChange={(event) => setSamplePayload(event.target.value)} style={textareaStyle} />
                    </label>
                    <label style={fieldStyle}>
                      sample_answer
                      <textarea aria-label="sample_answer" value={sampleAnswer} onChange={(event) => setSampleAnswer(event.target.value)} style={textareaStyle} />
                    </label>
                  </div>
                  <Button disabled={!activePromptId} loading={runningDryRun} onClick={() => void runDryRun()} style={{ marginTop: 'var(--space-sm)' }}>
                    运行 dry-run
                  </Button>
                  {dryRun ? (
                    <div style={dryRunResultStyle}>
                      <div><strong>{dryRun.provider}</strong></div>
                      <div>{dryRun.result.verdict} · {dryRun.result.overall_score}</div>
                      <div>{dryRun.result.reason}</div>
                      <ul style={{ margin: 'var(--space-sm) 0 0', paddingLeft: 18 }}>
                        {dryRun.result.dimensions.map((dimension) => (
                          <li key={dimension.name}>{dimension.name}: {dimension.score} · {dimension.reason}</li>
                        ))}
                      </ul>
                    </div>
                  ) : prompts.length === 0 ? (
                    <p style={mutedStyle}>当前任务未配置 AI Prompt</p>
                  ) : null}
                </div>
              </section>
              <Button onClick={() => void exportJSON(selected.id)} style={{ marginTop: 'var(--space-md)' }}>
                导出 approved JSON
              </Button>
              {exportRows.length > 0 ? (
                <pre style={{ marginTop: 'var(--space-md)', padding: 'var(--space-md)', background: 'var(--color-bg)', overflow: 'auto', maxHeight: 260 }}>
                  {JSON.stringify(exportRows.slice(0, 3), null, 2)}
                </pre>
              ) : null}
            </>
          ) : (
            <p>暂无任务</p>
          )}
        </section>
      </div>
    </div>
  )
}

function dimensionsToText(raw: string) {
  try {
    const parsed = JSON.parse(raw) as Array<{ name?: string }>
    return parsed.map((dimension) => dimension.name?.trim()).filter(Boolean).join('\n')
  } catch {
    return raw
  }
}

const panelStyle: React.CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border)',
  padding: 'var(--space-lg)',
}

const headingStyle: React.CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h2)',
  margin: 0,
}

const subHeadingStyle: React.CSSProperties = {
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-base)',
  margin: 0,
}

const aiPromptSectionStyle: React.CSSProperties = {
  marginTop: 'var(--space-lg)',
  paddingTop: 'var(--space-md)',
  borderTop: '1px solid var(--color-border-light)',
}

const formGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-sm)',
}

const dryRunGridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
  gap: 'var(--space-sm)',
}

const fieldStyle: React.CSSProperties = {
  display: 'grid',
  gap: 4,
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  minHeight: 36,
  border: '1px solid var(--color-border-light)',
  padding: '0 var(--space-sm)',
  boxSizing: 'border-box',
}

const textareaStyle: React.CSSProperties = {
  ...inputStyle,
  minHeight: 82,
  padding: 'var(--space-sm)',
  resize: 'vertical',
  fontFamily: 'var(--font-body)',
}

const dryRunPanelStyle: React.CSSProperties = {
  marginTop: 'var(--space-md)',
}

const dryRunResultStyle: React.CSSProperties = {
  marginTop: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
}

const alertStyle: React.CSSProperties = {
  marginTop: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-danger)',
  color: 'var(--color-danger)',
}

const mutedStyle: React.CSSProperties = {
  color: 'var(--color-text-secondary)',
}

const listButtonStyle: React.CSSProperties = {
  display: 'grid',
  gap: 4,
  width: '100%',
  marginTop: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  textAlign: 'left',
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
}

const activeListButtonStyle: React.CSSProperties = {
  ...listButtonStyle,
  borderColor: 'var(--color-accent)',
  background: 'var(--color-bg)',
}
