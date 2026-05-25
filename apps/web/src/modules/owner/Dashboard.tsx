import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react'
import { Button, Toast } from '@douyinfe/semi-ui'
import { apiDelete, apiGet, apiPost, apiPostRawJSON, type Task } from '../../shared/api/client'

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
  dryRunId?: number
  matchedExpected?: boolean
  result: {
    verdict: string
    overall_score: number
    dimensions: Array<{ name: string, score: number, reason: string }>
    reason: string
    model?: string
  }
}
type GoldenSample = {
  id: number
  taskId: number
  aiPromptId: number | null
  payload: unknown
  expectedAnswer: unknown
  expectedVerdict: string
  notes: string | null
  createdAt: string
}
type GoldenSamplesResponse = {
  samples: GoldenSample[]
}
type GoldenSampleCreateResponse = {
  sample: GoldenSample
}
type GoldenSampleBatchDryRunResult = {
  goldenSampleId: number
  status: 'succeeded' | 'failed'
  dryRunId?: number
  provider?: string
  result?: AIDryRunResult['result']
  matchedExpected?: boolean
  error?: string
}
type GoldenSampleBatchDryRunResponse = {
  results: GoldenSampleBatchDryRunResult[]
  summary: {
    total: number
    succeeded: number
    failed: number
  }
}
type AIDryRunHistoryItem = {
  id: number
  taskId: number
  aiPromptId: number
  goldenSampleId: number | null
  promptVersion: number
  expectedVerdict: string | null
  actualVerdict: string | null
  matchedExpected: boolean | null
  status: string
  errorMsg: string | null
  createdAt: string
  finishedAt: string | null
}
type AIDryRunHistoryResponse = {
  dryRuns: AIDryRunHistoryItem[]
}
type GoldenSampleRunRow = {
  sampleId: number
  expectedVerdict: string
  status: 'running' | 'succeeded' | 'failed'
  actualVerdict?: string
  matchedExpected?: boolean
  score?: number
  provider?: string
  model?: string
  reason?: string
  dryRunId?: number
  error?: string
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
  const [goldenSamples, setGoldenSamples] = useState<GoldenSample[]>([])
  const [goldenPayload, setGoldenPayload] = useState(defaultGoldenPayload)
  const [goldenExpectedAnswer, setGoldenExpectedAnswer] = useState(defaultGoldenExpectedAnswer)
  const [goldenExpectedVerdict, setGoldenExpectedVerdict] = useState(defaultGoldenExpectedVerdict)
  const [goldenNotes, setGoldenNotes] = useState('')
  const [goldenPromptChoice, setGoldenPromptChoice] = useState('active')
  const [goldenSampleError, setGoldenSampleError] = useState('')
  const [goldenSampleLoading, setGoldenSampleLoading] = useState(false)
  const [goldenSampleLoadFailed, setGoldenSampleLoadFailed] = useState(false)
  const [creatingGoldenSample, setCreatingGoldenSample] = useState(false)
  const [deletingGoldenSampleId, setDeletingGoldenSampleId] = useState<number | null>(null)
  const [goldenRunRows, setGoldenRunRows] = useState<Record<number, GoldenSampleRunRow>>({})
  const [dryRunHistory, setDryRunHistory] = useState<AIDryRunHistoryItem[]>([])
  const [dryRunHistorySampleFilter, setDryRunHistorySampleFilter] = useState('all')
  const [dryRunHistoryLoading, setDryRunHistoryLoading] = useState(false)
  const [dryRunHistoryError, setDryRunHistoryError] = useState('')
  const [promptError, setPromptError] = useState('')
  const [promptLoading, setPromptLoading] = useState(false)
  const [promptLoadFailed, setPromptLoadFailed] = useState(false)
  const [savingPrompt, setSavingPrompt] = useState(false)
  const [runningDryRun, setRunningDryRun] = useState(false)
  const [savingAIReviewSettings, setSavingAIReviewSettings] = useState(false)
  const promptLoadSeq = useRef(0)
  const goldenSampleLoadSeq = useRef(0)
  const selectedTaskIdRef = useRef<number | null>(null)
  const taskActionGeneration = useRef(0)
  const savePromptSeq = useRef(0)
  const aiReviewSettingsSeq = useRef(0)
  const dryRunSeq = useRef(0)
  const createGoldenSampleSeq = useRef(0)
  const deleteGoldenSampleSeq = useRef(0)
  const goldenSampleRunSeq = useRef(0)
  const dryRunHistorySeq = useRef(0)

  const resetGoldenSampleFormToDefaults = useCallback(() => {
    setGoldenPayload(defaultGoldenPayload)
    setGoldenExpectedAnswer(defaultGoldenExpectedAnswer)
    setGoldenExpectedVerdict(defaultGoldenExpectedVerdict)
    setGoldenNotes('')
    setGoldenPromptChoice('active')
  }, [])

  const loadTasks = useCallback(async () => {
    try {
      const data = await apiGet<TaskListResponse>('/tasks')
      setTasks(data)
      setSelected(data[0] ?? null)
      selectedTaskIdRef.current = data[0]?.id ?? null
      resetGoldenSampleFormToDefaults()
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载任务失败')
    }
  }, [resetGoldenSampleFormToDefaults])

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

  const loadGoldenSamples = useCallback(async (taskId: number) => {
    const requestSeq = goldenSampleLoadSeq.current + 1
    goldenSampleLoadSeq.current = requestSeq
    setGoldenSamples([])
    setGoldenRunRows({})
    setGoldenSampleError('')
    setGoldenSampleLoading(true)
    setGoldenSampleLoadFailed(false)
    setCreatingGoldenSample(false)
    setDeletingGoldenSampleId(null)
    try {
      const data = await apiGet<GoldenSamplesResponse>(`/tasks/${taskId}/golden-samples`)
      if (goldenSampleLoadSeq.current !== requestSeq || selectedTaskIdRef.current !== taskId) return
      setGoldenSamples(data.samples)
      setGoldenSampleError('')
      setGoldenSampleLoading(false)
      setGoldenSampleLoadFailed(false)
    } catch (error) {
      if (goldenSampleLoadSeq.current !== requestSeq || selectedTaskIdRef.current !== taskId) return
      setGoldenSamples([])
      setGoldenRunRows({})
      setGoldenSampleLoading(false)
      setGoldenSampleLoadFailed(true)
      setGoldenSampleError(error instanceof Error ? error.message : '加载 golden samples 失败')
    }
  }, [])

  const loadDryRunHistory = useCallback(async (taskId: number, sampleID: number | null) => {
    const requestSeq = dryRunHistorySeq.current + 1
    dryRunHistorySeq.current = requestSeq
    setDryRunHistoryLoading(true)
    setDryRunHistoryError('')
    const sampleQuery = sampleID ? `golden_sample_id=${sampleID}&` : ''
    try {
      const data = await apiGet<AIDryRunHistoryResponse>(`/tasks/${taskId}/ai-dry-runs?${sampleQuery}limit=10`)
      if (dryRunHistorySeq.current !== requestSeq || selectedTaskIdRef.current !== taskId) return
      setDryRunHistory(data.dryRuns)
      setDryRunHistoryLoading(false)
      setDryRunHistoryError('')
    } catch (error) {
      if (dryRunHistorySeq.current !== requestSeq || selectedTaskIdRef.current !== taskId) return
      setDryRunHistory([])
      setDryRunHistoryLoading(false)
      setDryRunHistoryError(error instanceof Error ? error.message : '加载 dry-run history 失败')
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadTasks()
  }, [loadTasks])

  useEffect(() => {
    if (selected) {
      selectedTaskIdRef.current = selected.id
      // eslint-disable-next-line react-hooks/set-state-in-effect
      void loadPrompts(selected.id)
      void loadGoldenSamples(selected.id)
    }
  }, [loadGoldenSamples, loadPrompts, selected])

  useEffect(() => {
    if (selected) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      void loadDryRunHistory(selected.id, dryRunHistorySampleID(dryRunHistorySampleFilter))
    }
  }, [dryRunHistorySampleFilter, loadDryRunHistory, selected])

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
    setExportRows([])
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
    setGoldenSamples([])
    setGoldenRunRows({})
    setGoldenSampleError('')
    setGoldenSampleLoading(true)
    setGoldenSampleLoadFailed(false)
    setCreatingGoldenSample(false)
    setDeletingGoldenSampleId(null)
    setDryRunHistory([])
    setDryRunHistorySampleFilter('all')
    setDryRunHistoryError('')
    setDryRunHistoryLoading(true)
    resetGoldenSampleFormToDefaults()
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
    const taskId = selected.id
    const guard = beginTaskAction(taskId, dryRunSeq)
    setRunningDryRun(true)
    setPromptError('')
    setDryRun(null)
    try {
      const body = buildAIDryRunBody(samplePayload, sampleAnswer)
      const data = await apiPostRawJSON<AIDryRunResult>(`/tasks/${taskId}/ai-prompts/${activePromptId}/dry-run`, body)
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

  async function createGoldenSample() {
    if (!selected || goldenSampleLoading || goldenSampleLoadFailed) return
    const guard = beginTaskAction(selected.id, createGoldenSampleSeq)
    setCreatingGoldenSample(true)
    setGoldenSampleError('')
    try {
      const body = buildGoldenSampleCreateBody({
        payload: goldenPayload,
        expectedAnswer: goldenExpectedAnswer,
        expectedVerdict: goldenExpectedVerdict,
        notes: goldenNotes,
        aiPromptId: resolveGoldenPromptId(),
      })
      const data = await apiPostRawJSON<GoldenSampleCreateResponse>(`/tasks/${selected.id}/golden-samples`, body)
      if (!isCurrentTaskAction(guard, createGoldenSampleSeq)) return
      setGoldenSamples((current) => [data.sample, ...current.filter((sample) => sample.id !== data.sample.id)])
      setGoldenNotes('')
      setGoldenSampleError('')
      Toast.success('Golden sample 已创建')
    } catch (error) {
      if (!isCurrentTaskAction(guard, createGoldenSampleSeq)) return
      const message = error instanceof Error ? error.message : '创建 golden sample 失败'
      setGoldenSampleError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, createGoldenSampleSeq)) {
        setCreatingGoldenSample(false)
      }
    }
  }

  async function deleteGoldenSample(sample: GoldenSample) {
    if (!selected || goldenSampleLoading || goldenSampleLoadFailed) return
    if (!window.confirm(`删除 golden sample #${sample.id}?`)) return
    const guard = beginTaskAction(selected.id, deleteGoldenSampleSeq)
    setDeletingGoldenSampleId(sample.id)
    setGoldenSampleError('')
    try {
      await apiDelete<{ deleted: boolean }>(`/tasks/${selected.id}/golden-samples/${sample.id}`)
      if (!isCurrentTaskAction(guard, deleteGoldenSampleSeq)) return
      setGoldenSamples((current) => current.filter((currentSample) => currentSample.id !== sample.id))
      setGoldenRunRows((current) => {
        const next = { ...current }
        delete next[sample.id]
        return next
      })
      Toast.success('Golden sample 已删除')
    } catch (error) {
      if (!isCurrentTaskAction(guard, deleteGoldenSampleSeq)) return
      const message = error instanceof Error ? error.message : '删除 golden sample 失败'
      setGoldenSampleError(message)
      Toast.error(message)
    } finally {
      if (isCurrentTaskAction(guard, deleteGoldenSampleSeq)) {
        setDeletingGoldenSampleId(null)
      }
    }
  }

  async function runGoldenSample(sample: GoldenSample) {
    if (!selected || goldenSampleLoading || goldenSampleLoadFailed) return
    const guard = beginTaskAction(selected.id, goldenSampleRunSeq)
    setGoldenSampleError('')
    await runGoldenSampleWithGuard(selected.id, sample, guard)
  }

  async function runAllGoldenSamples() {
    if (!selected || goldenSamples.length === 0 || goldenSampleLoading || goldenSampleLoadFailed) return
    const samples = goldenSamples
    const guard = beginTaskAction(selected.id, goldenSampleRunSeq)
    setGoldenSampleError('')
    setGoldenRunRows((current) => {
      const next = { ...current }
      for (const sample of samples) {
        next[sample.id] = {
          sampleId: sample.id,
          expectedVerdict: sample.expectedVerdict,
          status: 'running',
        }
      }
      return next
    })
    try {
      const data = await apiPost<GoldenSampleBatchDryRunResponse>(`/tasks/${selected.id}/golden-samples/dry-runs`, {
        sample_ids: samples.map((sample) => sample.id),
      })
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      const resultBySampleId = new Map(data.results.map((result) => [result.goldenSampleId, result]))
      setGoldenRunRows((current) => {
        const next = { ...current }
        for (const sample of samples) {
          next[sample.id] = goldenSampleBatchResultToRunRow(sample, resultBySampleId.get(sample.id))
        }
        return next
      })
      if (data.summary.failed > 0) {
        setGoldenSampleError(data.results.find((result) => result.status === 'failed')?.error || '部分 golden sample dry-run 失败')
      } else {
        setGoldenSampleError('')
      }
      void loadDryRunHistory(selected.id, dryRunHistorySampleID(dryRunHistorySampleFilter))
    } catch (error) {
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      const message = error instanceof Error ? error.message : 'golden sample batch dry-run 失败'
      setGoldenRunRows((current) => {
        const next = { ...current }
        for (const sample of samples) {
          next[sample.id] = {
            sampleId: sample.id,
            expectedVerdict: sample.expectedVerdict,
            status: 'failed',
            error: message,
          }
        }
        return next
      })
      setGoldenSampleError(message)
    }
  }

  async function runGoldenSampleWithGuard(taskId: number, sample: GoldenSample, guard: { taskId: number, generation: number, seq: number }) {
    setGoldenRunRows((current) => ({
      ...current,
      [sample.id]: {
        sampleId: sample.id,
        expectedVerdict: sample.expectedVerdict,
        status: 'running',
      },
    }))
    try {
      const data = await apiPost<AIDryRunResult>(`/tasks/${taskId}/golden-samples/${sample.id}/dry-run`, {})
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      setGoldenRunRows((current) => ({
        ...current,
        [sample.id]: {
          sampleId: sample.id,
          expectedVerdict: sample.expectedVerdict,
          status: 'succeeded',
          actualVerdict: data.result.verdict,
          matchedExpected: data.matchedExpected,
          score: data.result.overall_score,
          provider: data.provider,
          model: data.result.model,
          reason: data.result.reason,
          dryRunId: data.dryRunId,
        },
      }))
      void loadDryRunHistory(taskId, dryRunHistorySampleID(dryRunHistorySampleFilter))
    } catch (error) {
      if (!isCurrentTaskAction(guard, goldenSampleRunSeq)) return
      const message = error instanceof Error ? error.message : 'golden sample dry-run 失败'
      setGoldenRunRows((current) => ({
        ...current,
        [sample.id]: {
          sampleId: sample.id,
          expectedVerdict: sample.expectedVerdict,
          status: 'failed',
          error: message,
        },
      }))
      setGoldenSampleError(message)
      void loadDryRunHistory(taskId, dryRunHistorySampleID(dryRunHistorySampleFilter))
    }
  }

  function resolveGoldenPromptId() {
    if (goldenPromptChoice === 'none') return null
    if (goldenPromptChoice === 'active') return activePromptId
    const promptId = Number(goldenPromptChoice)
    return Number.isFinite(promptId) && promptId > 0 ? promptId : null
  }

  function promptVersionLabel(promptId: number | null) {
    if (!promptId) return '未绑定 prompt'
    const prompt = prompts.find((candidate) => candidate.id === promptId)
    return prompt ? `Prompt v${prompt.version} (#${prompt.id})` : `Prompt #${promptId}`
  }

  const promptActionDisabled = promptLoading || promptLoadFailed
  const goldenActionDisabled = goldenSampleLoading || goldenSampleLoadFailed
  const anyGoldenRunRunning = Object.values(goldenRunRows).some((row) => row.status === 'running')
  const goldenRunResults = goldenSamples.map((sample) => goldenRunRows[sample.id]).filter((row): row is GoldenSampleRunRow => Boolean(row))
  const selectedHistorySampleID = dryRunHistorySampleID(dryRunHistorySampleFilter)
  const dryRunHistorySummary = summarizeDryRunHistory(dryRunHistory)

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
              <div style={{ marginTop: 'var(--space-md)' }}>
                <a href={`/owner/tasks/${selected.id}/templates`} style={templateDesignerLinkStyle}>模板 Designer</a>
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
                <div style={goldenSampleSectionStyle}>
                  <div style={aiSettingsRowStyle}>
                    <h3 style={subHeadingStyle}>Golden Samples</h3>
                    <Button disabled={goldenActionDisabled || goldenSamples.length === 0 || anyGoldenRunRunning} onClick={() => void runAllGoldenSamples()}>
                      Run all visible samples
                    </Button>
                  </div>
                  {goldenSampleError ? <div role="alert" style={alertStyle}>{goldenSampleError}</div> : null}
                  <div style={dryRunGridStyle}>
                    <label style={fieldStyle}>
                      golden_sample_payload
                      <textarea aria-label="golden_sample_payload" value={goldenPayload} onChange={(event) => setGoldenPayload(event.target.value)} style={textareaStyle} />
                    </label>
                    <label style={fieldStyle}>
                      golden_sample_expected_answer
                      <textarea aria-label="golden_sample_expected_answer" value={goldenExpectedAnswer} onChange={(event) => setGoldenExpectedAnswer(event.target.value)} style={textareaStyle} />
                    </label>
                  </div>
                  <div style={formGridStyle}>
                    <label style={fieldStyle}>
                      expected_verdict
                      <select aria-label="golden_sample_expected_verdict" value={goldenExpectedVerdict} onChange={(event) => setGoldenExpectedVerdict(event.target.value)} style={inputStyle}>
                        <option value="pass">pass</option>
                        <option value="reject">reject</option>
                        <option value="uncertain">uncertain</option>
                      </select>
                    </label>
                    <label style={fieldStyle}>
                      ai_prompt
                      <select aria-label="golden_sample_prompt" value={goldenPromptChoice} onChange={(event) => setGoldenPromptChoice(event.target.value)} style={inputStyle}>
                        <option value="active">当前 active prompt{activePromptId ? ` (#${activePromptId})` : ''}</option>
                        <option value="none">不绑定 prompt</option>
                        {prompts.map((prompt) => (
                          <option key={prompt.id} value={String(prompt.id)}>Prompt v{prompt.version} #{prompt.id}</option>
                        ))}
                      </select>
                    </label>
                    <label style={fieldStyle}>
                      notes
                      <input aria-label="golden_sample_notes" value={goldenNotes} onChange={(event) => setGoldenNotes(event.target.value)} style={inputStyle} />
                    </label>
                  </div>
                  <Button disabled={goldenActionDisabled || creatingGoldenSample} loading={creatingGoldenSample} onClick={() => void createGoldenSample()} style={{ marginTop: 'var(--space-sm)' }}>
                    创建 golden sample
                  </Button>
                  {goldenSampleLoading ? (
                    <p style={mutedStyle}>加载 golden samples...</p>
                  ) : goldenSamples.length === 0 ? (
                    <p style={mutedStyle}>暂无 golden samples</p>
                  ) : (
                    <div style={goldenSampleListStyle}>
                      {goldenSamples.map((sample) => {
                        const runRow = goldenRunRows[sample.id]
                        return (
                          <div key={sample.id} style={goldenSampleItemStyle}>
                            <div style={goldenSampleHeaderStyle}>
                              <strong>#{sample.id} · {sample.expectedVerdict}</strong>
                              <span>{promptVersionLabel(sample.aiPromptId)}</span>
                              <span>{formatDateTime(sample.createdAt)}</span>
                            </div>
                            {sample.notes ? <div style={mutedStyle}>{sample.notes}</div> : null}
                            <div style={dryRunGridStyle}>
                              <pre style={compactPreviewStyle}>{formatCompactJSON(sample.payload)}</pre>
                              <pre style={compactPreviewStyle}>{formatCompactJSON(sample.expectedAnswer)}</pre>
                            </div>
                            <div style={goldenSampleActionsStyle}>
                              <Button aria-label={`Run golden sample ${sample.id}`} disabled={goldenActionDisabled || runRow?.status === 'running' || anyGoldenRunRunning} loading={runRow?.status === 'running'} onClick={() => void runGoldenSample(sample)}>
                                Run
                              </Button>
                              <Button aria-label={`查看 golden sample ${sample.id} history`} disabled={dryRunHistoryLoading} onClick={() => setDryRunHistorySampleFilter(String(sample.id))}>
                                History
                              </Button>
                              <Button aria-label={`删除 golden sample ${sample.id}`} disabled={goldenActionDisabled || deletingGoldenSampleId === sample.id || anyGoldenRunRunning} loading={deletingGoldenSampleId === sample.id} onClick={() => void deleteGoldenSample(sample)}>
                                删除
                              </Button>
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  )}
                  {goldenRunResults.length > 0 ? (
                    <div style={resultTableWrapStyle}>
                      <table style={resultTableStyle}>
                        <thead>
                          <tr>
                            <th style={resultCellStyle}>sample</th>
                            <th style={resultCellStyle}>expected</th>
                            <th style={resultCellStyle}>actual</th>
                            <th style={resultCellStyle}>matched</th>
                            <th style={resultCellStyle}>score</th>
                            <th style={resultCellStyle}>provider</th>
                            <th style={resultCellStyle}>dryRunId</th>
                            <th style={resultCellStyle}>reason / error</th>
                          </tr>
                        </thead>
                        <tbody>
                          {goldenRunResults.map((row) => (
                            <tr key={row.sampleId}>
                              <td style={resultCellStyle}>#{row.sampleId}</td>
                              <td style={resultCellStyle}>{row.expectedVerdict}</td>
                              <td style={resultCellStyle}>{row.actualVerdict || row.status}</td>
                              <td style={resultCellStyle}>{row.matchedExpected === undefined ? '-' : row.matchedExpected ? 'matched' : 'mismatch'}</td>
                              <td style={resultCellStyle}>{row.score ?? '-'}</td>
                              <td style={resultCellStyle}>{[row.provider, row.model].filter(Boolean).join(' / ') || '-'}</td>
                              <td style={resultCellStyle}>{row.dryRunId ?? '-'}</td>
                              <td style={resultCellStyle}>{row.error || row.reason || '-'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : null}
                  <div style={historyPanelStyle}>
                    <div style={aiSettingsRowStyle}>
                      <h3 style={subHeadingStyle}>Dry-run History</h3>
                      <Button disabled={!selected || dryRunHistoryLoading} loading={dryRunHistoryLoading} onClick={() => selected && void loadDryRunHistory(selected.id, selectedHistorySampleID)}>
                        Refresh history
                      </Button>
                    </div>
                    <label style={{ ...fieldStyle, marginTop: 'var(--space-sm)' }}>
                      history_sample_filter
                      <select aria-label="dry_run_history_sample_filter" value={dryRunHistorySampleFilter} onChange={(event) => setDryRunHistorySampleFilter(event.target.value)} style={inputStyle}>
                        <option value="all">最近全部 dry-runs</option>
                        {goldenSamples.map((sample) => (
                          <option key={sample.id} value={String(sample.id)}>Golden sample #{sample.id}</option>
                        ))}
                      </select>
                    </label>
                    {dryRunHistoryError ? <p style={errorTextStyle}>{dryRunHistoryError}</p> : null}
                    {dryRunHistory.length > 0 ? (
                      <div style={historySummaryStyle}>
                        <span>total {dryRunHistorySummary.total}</span>
                        <span>matched {dryRunHistorySummary.matched}</span>
                        <span>mismatch {dryRunHistorySummary.mismatch}</span>
                        <span>failed {dryRunHistorySummary.failed}</span>
                      </div>
                    ) : null}
                    {dryRunHistoryLoading ? (
                      <p style={mutedStyle}>加载 dry-run history...</p>
                    ) : dryRunHistory.length === 0 ? (
                      <p style={mutedStyle}>暂无 dry-run history</p>
                    ) : (
                      <div style={resultTableWrapStyle}>
                        <table style={resultTableStyle}>
                          <thead>
                            <tr>
                              <th style={resultCellStyle}>dryRunId</th>
                              <th style={resultCellStyle}>sample</th>
                              <th style={resultCellStyle}>expected</th>
                              <th style={resultCellStyle}>actual</th>
                              <th style={resultCellStyle}>matched</th>
                              <th style={resultCellStyle}>status / error</th>
                              <th style={resultCellStyle}>prompt</th>
                              <th style={resultCellStyle}>finished</th>
                            </tr>
                          </thead>
                          <tbody>
                            {dryRunHistory.map((run) => (
                              <tr key={run.id}>
                                <td style={resultCellStyle}>#{run.id}</td>
                                <td style={resultCellStyle}>{run.goldenSampleId ? `#${run.goldenSampleId}` : '-'}</td>
                                <td style={resultCellStyle}>{run.expectedVerdict || '-'}</td>
                                <td style={resultCellStyle}>{run.actualVerdict || '-'}</td>
                                <td style={resultCellStyle}>{formatMatched(run.matchedExpected)}</td>
                                <td style={resultCellStyle}>{[run.status, run.errorMsg].filter(Boolean).join(' · ')}</td>
                                <td style={resultCellStyle}>v{run.promptVersion} #{run.aiPromptId}</td>
                                <td style={resultCellStyle}>{formatDateTime(run.finishedAt || run.createdAt)}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>
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
const defaultGoldenPayload = '{"prompt":"示例题目"}'
const defaultGoldenExpectedAnswer = '{"summary":"示例答案"}'
const defaultGoldenExpectedVerdict = 'pass'

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

function buildGoldenSampleCreateBody(input: { payload: string, expectedAnswer: string, expectedVerdict: string, notes: string, aiPromptId: number | null }) {
  const payload = normalizeJSONInput(input.payload, 'payload')
  const expectedAnswer = normalizeJSONInput(input.expectedAnswer, 'expected_answer')
  const fields = [
    `"payload":${payload}`,
    `"expected_answer":${expectedAnswer}`,
    `"expected_verdict":${JSON.stringify(input.expectedVerdict)}`,
  ]
  const notes = input.notes.trim()
  if (notes) {
    fields.push(`"notes":${JSON.stringify(notes)}`)
  }
  if (input.aiPromptId) {
    fields.push(`"ai_prompt_id":${input.aiPromptId}`)
  }
  return `{${fields.join(',')}}`
}

function buildAIDryRunBody(payloadInput: string, answerInput: string) {
  const payload = normalizeJSONInput(payloadInput, 'payload')
  const answer = normalizeJSONInput(answerInput, 'answer')
  return `{"payload":${payload},"answer":${answer}}`
}

function goldenSampleBatchResultToRunRow(sample: GoldenSample, result: GoldenSampleBatchDryRunResult | undefined): GoldenSampleRunRow {
  if (result?.status === 'succeeded' && result.result) {
    return {
      sampleId: sample.id,
      expectedVerdict: sample.expectedVerdict,
      status: 'succeeded',
      actualVerdict: result.result.verdict,
      matchedExpected: result.matchedExpected,
      score: result.result.overall_score,
      provider: result.provider,
      model: result.result.model,
      reason: result.result.reason,
      dryRunId: result.dryRunId,
    }
  }
  return {
    sampleId: sample.id,
    expectedVerdict: sample.expectedVerdict,
    status: 'failed',
    dryRunId: result?.dryRunId,
    error: result?.error || 'missing dry-run result',
  }
}

function dryRunHistorySampleID(value: string) {
  if (value === 'all') return null
  const sampleID = Number(value)
  return Number.isFinite(sampleID) && sampleID > 0 ? sampleID : null
}

function normalizeJSONInput(raw: string, label: string) {
  const trimmed = raw.trim()
  if (!trimmed || trimmed === 'null') {
    throw new Error(`${label} is required`)
  }
  JSON.parse(trimmed)
  return trimmed
}

function formatCompactJSON(value: unknown) {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function formatDateTime(value: string) {
  if (!value) return '-'
  const time = new Date(value)
  if (Number.isNaN(time.getTime())) return value
  return time.toLocaleString()
}

function formatMatched(value: boolean | null | undefined) {
  if (value === true) return 'matched'
  if (value === false) return 'mismatch'
  return '-'
}

function summarizeDryRunHistory(runs: AIDryRunHistoryItem[]) {
  return runs.reduce((summary, run) => {
    summary.total += 1
    if (run.matchedExpected === true) {
      summary.matched += 1
    } else if (run.matchedExpected === false) {
      summary.mismatch += 1
    }
    if (run.status === 'failed') {
      summary.failed += 1
    }
    return summary
  }, { total: 0, matched: 0, mismatch: 0, failed: 0 })
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

const goldenSampleSectionStyle: React.CSSProperties = {
  marginTop: 'var(--space-lg)',
  paddingTop: 'var(--space-md)',
  borderTop: '1px solid var(--color-border-light)',
}

const goldenSampleListStyle: React.CSSProperties = {
  display: 'grid',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-md)',
}

const goldenSampleItemStyle: React.CSSProperties = {
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
}

const goldenSampleHeaderStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 'var(--space-sm)',
  alignItems: 'center',
  justifyContent: 'space-between',
  color: 'var(--color-text-secondary)',
}

const goldenSampleActionsStyle: React.CSSProperties = {
  display: 'flex',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-sm)',
}

const compactPreviewStyle: React.CSSProperties = {
  margin: 'var(--space-sm) 0 0',
  padding: 'var(--space-sm)',
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border-light)',
  maxHeight: 120,
  overflow: 'auto',
  whiteSpace: 'pre-wrap',
}

const resultTableWrapStyle: React.CSSProperties = {
  marginTop: 'var(--space-md)',
  overflowX: 'auto',
}

const resultTableStyle: React.CSSProperties = {
  width: '100%',
  borderCollapse: 'collapse',
  fontSize: 'var(--text-sm)',
}

const resultCellStyle: React.CSSProperties = {
  border: '1px solid var(--color-border-light)',
  padding: 'var(--space-sm)',
  textAlign: 'left',
  verticalAlign: 'top',
}

const historyPanelStyle: React.CSSProperties = {
  marginTop: 'var(--space-md)',
  paddingTop: 'var(--space-md)',
  borderTop: '1px solid var(--color-border-light)',
}

const historySummaryStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 'var(--space-sm)',
  marginTop: 'var(--space-sm)',
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}

const alertStyle: React.CSSProperties = {
  marginTop: 'var(--space-sm)',
  padding: 'var(--space-sm)',
  border: '1px solid var(--color-danger)',
  color: 'var(--color-danger)',
}

const errorTextStyle: React.CSSProperties = {
  color: 'var(--color-danger)',
  margin: 'var(--space-sm) 0 0',
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

const templateDesignerLinkStyle: React.CSSProperties = {
  display: 'inline-flex',
  minHeight: 36,
  alignItems: 'center',
  padding: '0 var(--space-md)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-text)',
  background: 'var(--color-surface)',
  textDecoration: 'none',
  fontFamily: 'var(--font-body)',
}
