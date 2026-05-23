import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react'
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
type AIReviewSettingsResponse = {
  aiReviewEnabled: boolean
  activePromptId: number | null
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
  const [aiReviewEnabled, setAIReviewEnabled] = useState(false)
  const [promptTemplate, setPromptTemplate] = useState(defaultPromptTemplate)
  const [dimensionsText, setDimensionsText] = useState(defaultDimensionsJSON)
  const [passThreshold, setPassThreshold] = useState(defaultPassThreshold)
  const [uncertainMin, setUncertainMin] = useState(defaultUncertainMin)
  const [model, setModel] = useState(defaultModel)
  const [samplePayload, setSamplePayload] = useState('{"prompt":"示例题目"}')
  const [sampleAnswer, setSampleAnswer] = useState('{"summary":"示例答案"}')
  const [dryRun, setDryRun] = useState<AIDryRunResult | null>(null)
  const [promptError, setPromptError] = useState('')
  const [promptLoading, setPromptLoading] = useState(false)
  const [promptLoadFailed, setPromptLoadFailed] = useState(false)
  const [savingPrompt, setSavingPrompt] = useState(false)
  const [runningDryRun, setRunningDryRun] = useState(false)
  const [savingAIReviewSettings, setSavingAIReviewSettings] = useState(false)
  const promptLoadSeq = useRef(0)
  const selectedTaskIdRef = useRef<number | null>(null)
  const taskActionGeneration = useRef(0)
  const savePromptSeq = useRef(0)
  const aiReviewSettingsSeq = useRef(0)
  const dryRunSeq = useRef(0)

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<TaskListResponse>('/tasks')
      setTasks(data)
      setSelected(data[0] ?? null)
      selectedTaskIdRef.current = data[0]?.id ?? null
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载任务失败')
    }
  }, [])

  const fillPromptForm = useCallback((prompt: AIPromptConfig) => {
    setPromptTemplate(prompt.promptTemplate)
    setDimensionsText(formatDimensionsJSON(prompt.dimensions))
    setPassThreshold(String(prompt.passThreshold))
    setUncertainMin(String(prompt.uncertainMin))
    setModel(prompt.model)
  }, [])

  const resetPromptFormToDefaults = useCallback(() => {
    setPromptTemplate(defaultPromptTemplate)
    setDimensionsText(defaultDimensionsJSON)
    setPassThreshold(defaultPassThreshold)
    setUncertainMin(defaultUncertainMin)
    setModel(defaultModel)
  }, [])

  const loadPrompts = useCallback(async (taskId: number) => {
    const requestSeq = promptLoadSeq.current + 1
    promptLoadSeq.current = requestSeq
    taskActionGeneration.current += 1
    setPrompts([])
    setActivePromptId(null)
    setAIReviewEnabled(false)
    setDryRun(null)
    setPromptError('')
    setPromptLoading(true)
    setPromptLoadFailed(false)
    setSavingPrompt(false)
    setRunningDryRun(false)
    setSavingAIReviewSettings(false)
    try {
      const data = await apiGet<AIPromptsResponse>(`/tasks/${taskId}/ai-prompts`)
      if (promptLoadSeq.current !== requestSeq) return
      setPrompts(data.prompts)
      setActivePromptId(data.activePromptId)
      setAIReviewEnabled(data.aiReviewEnabled)
      setDryRun(null)
      setPromptError('')
      setPromptLoading(false)
      setPromptLoadFailed(false)
      const latest = data.prompts[0]
      if (latest) {
        fillPromptForm(latest)
      } else {
        resetPromptFormToDefaults()
      }
    } catch (error) {
      if (promptLoadSeq.current !== requestSeq) return
      setPrompts([])
      setActivePromptId(null)
      setAIReviewEnabled(false)
      setDryRun(null)
      resetPromptFormToDefaults()
      setPromptLoading(false)
      setPromptLoadFailed(true)
      setPromptError(error instanceof Error ? error.message : '加载 AI Prompt 失败')
    }
  }, [fillPromptForm, resetPromptFormToDefaults])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTasks()
  }, [loadTasks])

  useEffect(() => {
    if (selected) {
      selectedTaskIdRef.current = selected.id
      // eslint-disable-next-line react-hooks/set-state-in-effect
      void loadPrompts(selected.id)
    }
  }, [loadPrompts, selected])

  function beginTaskAction(taskId: number, seqRef: MutableRefObject<number>) {
    seqRef.current += 1
    return { taskId, generation: taskActionGeneration.current, seq: seqRef.current }
  }

  function isCurrentTaskAction(guard: { taskId: number, generation: number, seq: number }, seqRef: MutableRefObject<number>) {
    return selectedTaskIdRef.current === guard.taskId && taskActionGeneration.current === guard.generation && seqRef.current === guard.seq
  }

  function selectTask(task: Task) {
    if (task.id === selectedTaskIdRef.current) return
    selectedTaskIdRef.current = task.id
    taskActionGeneration.current += 1
    setSelected(task)
  }

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
    if (!selected || promptLoading || promptLoadFailed) return
    const guard = beginTaskAction(selected.id, savePromptSeq)
    setSavingPrompt(true)
    setPromptError('')
    try {
      const dimensions = parseDimensionsInput(dimensionsText)
      const data = await apiPost<{ prompt: AIPromptConfig, activePromptId: number }>(`/tasks/${selected.id}/ai-prompts`, {
        prompt_template: promptTemplate,
        dimensions,
        pass_threshold: Number(passThreshold),
        uncertain_min: Number(uncertainMin),
        model,
      })
      if (!isCurrentTaskAction(guard, savePromptSeq)) return
      setPrompts((current) => [data.prompt, ...current])
      setActivePromptId(data.activePromptId)
      fillPromptForm(data.prompt)
      Toast.success('AI Prompt 已保存')
    } catch (error) {
      if (!isCurrentTaskAction(guard, savePromptSeq)) return
      const message = error instanceof Error ? error.message : '保存 AI Prompt 失败'
      setPromptError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, savePromptSeq)) {
        setSavingPrompt(false)
      }
    }
  }

  async function updateAIReviewSettings(enabled: boolean) {
    if (!selected || promptLoading || promptLoadFailed) return
    const taskId = selected.id
    const guard = beginTaskAction(taskId, aiReviewSettingsSeq)
    setSavingAIReviewSettings(true)
    setPromptError('')
    try {
      const data = await apiPost<AIReviewSettingsResponse>(`/tasks/${taskId}/ai-review-settings`, { enabled })
      if (!isCurrentTaskAction(guard, aiReviewSettingsSeq)) return
      setAIReviewEnabled(data.aiReviewEnabled)
      setActivePromptId(data.activePromptId)
      setTasks((current) => current.map((task) => task.id === taskId ? { ...task, aiReviewEnabled: data.aiReviewEnabled, aiPromptId: data.activePromptId } : task))
      Toast.success(enabled ? 'AI review 已启用' : 'AI review 已关闭')
    } catch (error) {
      if (!isCurrentTaskAction(guard, aiReviewSettingsSeq)) return
      const message = error instanceof Error ? error.message : '更新 AI review 设置失败'
      setPromptError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, aiReviewSettingsSeq)) {
        setSavingAIReviewSettings(false)
      }
    }
  }

  async function runDryRun() {
    if (!selected || !activePromptId || promptLoading || promptLoadFailed) return
    const guard = beginTaskAction(selected.id, dryRunSeq)
    setRunningDryRun(true)
    setPromptError('')
    setDryRun(null)
    try {
      const data = await apiPost<AIDryRunResult>(`/tasks/${selected.id}/ai-prompts/${activePromptId}/dry-run`, {
        payload: JSON.parse(samplePayload) as Record<string, unknown>,
        answer: JSON.parse(sampleAnswer) as Record<string, unknown>,
      })
      if (!isCurrentTaskAction(guard, dryRunSeq)) return
      setDryRun(data)
      Toast.success('dry-run 完成')
    } catch (error) {
      if (!isCurrentTaskAction(guard, dryRunSeq)) return
      const message = error instanceof Error ? error.message : 'dry-run 失败'
      setDryRun(null)
      setPromptError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, dryRunSeq)) {
        setRunningDryRun(false)
      }
    }
  }

  const promptActionDisabled = promptLoading || promptLoadFailed

  return (
    <div>
      <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)' }}>Owner 任务负责人</h1>
      <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)' }}>任务管理 · baseline · 数据导出</p>

      <div style={{ display: 'grid', gridTemplateColumns: '280px minmax(0, 1fr)', gap: 'var(--space-lg)', marginTop: 'var(--space-lg)' }}>
        <section style={panelStyle}>
          <h2 style={headingStyle}>任务</h2>
          {tasks.map((task) => (
            <button key={task.id} onClick={() => selectTask(task)} style={task.id === selected?.id ? activeListButtonStyle : listButtonStyle}>
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
                <div style={aiSettingsRowStyle}>
                  <span>AI review: {aiReviewEnabled ? '已启用' : '已关闭'}</span>
                  {aiReviewEnabled ? (
                    <Button disabled={promptActionDisabled || savingAIReviewSettings} loading={savingAIReviewSettings} onClick={() => void updateAIReviewSettings(false)}>
                      关闭 AI review
                    </Button>
                  ) : (
                    <Button disabled={!activePromptId || promptActionDisabled || savingAIReviewSettings} loading={savingAIReviewSettings} onClick={() => void updateAIReviewSettings(true)}>
                      启用 AI review
                    </Button>
                  )}
                </div>
                {!activePromptId ? <p style={mutedStyle}>保存 AI Prompt 后才能启用 AI review</p> : null}
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
                <Button disabled={promptActionDisabled || savingPrompt} loading={savingPrompt} theme="solid" onClick={() => void savePrompt()} style={{ marginTop: 'var(--space-md)' }}>
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
                  <Button disabled={!activePromptId || promptActionDisabled || runningDryRun} loading={runningDryRun} onClick={() => void runDryRun()} style={{ marginTop: 'var(--space-sm)' }}>
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

const defaultPromptTemplate = '请根据 payload 和 answer 完成结构化预审。'
const defaultPassThreshold = '80'
const defaultUncertainMin = '60'
const defaultModel = ''

const defaultDimensionsJSON = JSON.stringify([
  { name: '相关性', description: '是否相关', weight: 1 },
  { name: '准确性', description: '是否准确', weight: 1 },
  { name: '格式合规', description: '是否符合格式', weight: 1 },
], null, 2)

function formatDimensionsJSON(raw: string) {
  try {
    return JSON.stringify(JSON.parse(raw) as unknown, null, 2)
  } catch {
    return raw
  }
}

function parseDimensionsInput(raw: string): Array<Record<string, unknown>> {
  const parsed = JSON.parse(raw) as unknown
  if (!Array.isArray(parsed)) {
    throw new Error('dimensions must be a JSON array')
  }
  return parsed.filter((dimension): dimension is Record<string, unknown> => {
    return typeof dimension === 'object' && dimension !== null
  })
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

const aiSettingsRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-sm)',
  color: 'var(--color-text-secondary)',
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
