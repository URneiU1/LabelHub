import { useEffect, useRef, useState } from 'react'
import { Toast } from '@douyinfe/semi-ui'
import {
  batchUpdateItems,
  importItems,
  importItemsFile,
  listTaskItems,
  previewItem,
  type PreviewItem,
} from '../../shared/api/client'

interface ImportPanelProps {
  taskId: number
  // 导入成功后通知父组件刷新任务(total_items 会变)。
  onImported: () => void
}

type EditableItem = {
  itemId: number
  externalId: string | null
  status: string
  payloadDraft: string
}

// 批量编辑列表每页拉取的条数(后端上限 100)。
const ITEMS_PAGE_SIZE = 50

export default function ImportPanel({ taskId, onImported }: ImportPanelProps) {
  const [uploading, setUploading] = useState(false)
  // 常驻显示最近一次导入结果(成功/失败),不随 Toast 消失,方便演示与确认。
  const [lastResult, setLastResult] = useState<{ ok: boolean, text: string } | null>(null)
  const [jsonText, setJsonText] = useState('')
  const [importingJson, setImportingJson] = useState(false)
  const [preview, setPreview] = useState<PreviewItem | null>(null)
  const [loadingPreview, setLoadingPreview] = useState(false)
  const [editableItems, setEditableItems] = useState<EditableItem[]>([])
  const [savingBatch, setSavingBatch] = useState(false)
  const [loadingItems, setLoadingItems] = useState(false)
  const [itemsCursor, setItemsCursor] = useState('')
  const [itemsHasMore, setItemsHasMore] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // 切任务时清空导入面板的本地草稿状态。
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- 切任务时重置本地导入草稿
    setJsonText('')
    setLastResult(null)
    setPreview(null)
    setEditableItems([])
    setItemsCursor('')
    setItemsHasMore(false)
  }, [taskId])

  async function handleFile(file: File | undefined) {
    if (!file) return
    setUploading(true)
    try {
      const result = await importItemsFile(taskId, file)
      setLastResult({ ok: true, text: `已导入 ${result.imported} 条(${result.format})` })
      Toast.success(`已导入 ${result.imported} 条(${result.format})`)
      onImported()
    } catch (error) {
      const message = error instanceof Error ? error.message : '文件导入失败'
      setLastResult({ ok: false, text: `导入失败:${message}` })
      Toast.error(message)
    } finally {
      setUploading(false)
      if (fileInputRef.current) {
        fileInputRef.current.value = ''
      }
    }
  }

  async function importFromJson() {
    if (savingBatch || importingJson) return
    let parsed: unknown
    try {
      parsed = JSON.parse(jsonText)
    } catch {
      Toast.error('JSON 解析失败,请检查格式')
      return
    }
    const items = Array.isArray(parsed) ? parsed : (parsed as { items?: unknown }).items
    if (!Array.isArray(items)) {
      Toast.error('需要一个 JSON 数组,或 {"items":[...]} 结构')
      return
    }
    setImportingJson(true)
    try {
      const result = await importItems(taskId, items as Array<Record<string, unknown>>)
      setLastResult({ ok: true, text: `已导入 ${result.imported} 条(JSON)` })
      Toast.success(`已导入 ${result.imported} 条`)
      setJsonText('')
      onImported()
    } catch (error) {
      const message = error instanceof Error ? error.message : 'JSON 导入失败'
      setLastResult({ ok: false, text: `导入失败:${message}` })
      Toast.error(message)
    } finally {
      setImportingJson(false)
    }
  }

  async function loadPreview() {
    setLoadingPreview(true)
    try {
      const item = await previewItem(taskId)
      setPreview(item)
      if (!item) {
        Toast.info('当前没有可预览的可用题目')
      }
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载题目预览失败')
    } finally {
      setLoadingPreview(false)
    }
  }

  // 从 GET /tasks/:id/items 游标分页拉取本任务的全部题目进可编辑列表。
  // reset=true 重载第一页(替换列表);reset=false 追加下一页,且保留已存在 itemId 的本地草稿。
  async function loadItems(reset: boolean) {
    if (loadingItems) return
    setLoadingItems(true)
    try {
      const page = await listTaskItems(taskId, {
        cursor: reset ? undefined : itemsCursor || undefined,
        limit: ITEMS_PAGE_SIZE,
      })
      setEditableItems((current) => {
        const rows: EditableItem[] = page.items.map((it) => ({
          itemId: it.id,
          externalId: it.externalId,
          status: it.status,
          payloadDraft: JSON.stringify(it.payload, null, 2),
        }))
        if (reset) {
          return rows
        }
        const seen = new Set(current.map((row) => row.itemId))
        return [...current, ...rows.filter((row) => !seen.has(row.itemId))]
      })
      setItemsCursor(page.nextCursor)
      setItemsHasMore(page.hasMore)
      if (reset && page.items.length === 0) {
        Toast.info('该任务还没有题目,请先导入')
      }
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '加载题目列表失败')
    } finally {
      setLoadingItems(false)
    }
  }

  function updateDraft(itemId: number, value: string) {
    setEditableItems((current) => current.map((row) => row.itemId === itemId ? { ...row, payloadDraft: value } : row))
  }

  function removeDraft(itemId: number) {
    setEditableItems((current) => current.filter((row) => row.itemId !== itemId))
  }

  async function saveBatch() {
    if (savingBatch || editableItems.length === 0) return
    const items: Array<{ itemId: number, payload: unknown }> = []
    for (const row of editableItems) {
      try {
        items.push({ itemId: row.itemId, payload: JSON.parse(row.payloadDraft) })
      } catch {
        Toast.error(`题目 #${row.itemId} 的 payload 不是合法 JSON`)
        return
      }
    }
    setSavingBatch(true)
    try {
      const result = await batchUpdateItems(taskId, items)
      Toast.success(`已更新 ${result.updated}/${result.requested} 条题目`)
      onImported()
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '批量更新失败')
    } finally {
      setSavingBatch(false)
    }
  }

  return (
    <div className="lh-vflex" style={{ gap: 18 }}>
      <div className="taskform__field">
        <label className="taskform__label">文件导入(.json / .jsonl / .xlsx)</label>
        <input
          ref={fileInputRef}
          type="file"
          aria-label="import_file"
          accept=".json,.jsonl,.ndjson,.xlsx"
          disabled={uploading}
          onChange={(event) => void handleFile(event.target.files?.[0])}
          className="taskform__input"
        />
        {uploading ? <span className="lh-muted lh-text-12">上传解析中…</span> : null}
      </div>

      {lastResult ? (
        <div
          aria-label="import_result"
          role="status"
          style={{
            padding: '10px 14px',
            borderRadius: 8,
            fontSize: 13,
            fontWeight: 600,
            border: '1px solid',
            borderColor: lastResult.ok ? '#bbf7d0' : '#fecaca',
            background: lastResult.ok ? '#f0fdf4' : '#fef2f2',
            color: lastResult.ok ? '#15803d' : '#b91c1c',
          }}
        >
          {lastResult.ok ? '✓ ' : '✕ '}{lastResult.text}
        </div>
      ) : null}

      <div className="taskform__field">
        <label className="taskform__label">JSON 粘贴导入</label>
        <textarea
          aria-label="import_json_text"
          className="taskform__textarea"
          value={jsonText}
          onChange={(event) => setJsonText(event.target.value)}
          placeholder='[{"id":"a-1","text":"示例题目"}]'
        />
        <div className="lh-hflex" style={{ marginTop: 6 }}>
          <button type="button" aria-label="导入 JSON" disabled={importingJson || !jsonText.trim()} className="lh-btn lh-btn--primary" onClick={() => void importFromJson()}>
            {importingJson ? '导入中…' : '导入 JSON'}
          </button>
        </div>
      </div>

      <div className="taskform__field">
        <div className="lh-hflex" style={{ justifyContent: 'space-between' }}>
          <label className="taskform__label" style={{ marginBottom: 0 }}>题目预览</label>
          <button type="button" aria-label="加载题目预览" disabled={loadingPreview} className="lh-btn lh-btn--sm" onClick={() => void loadPreview()}>
            {loadingPreview ? '加载中…' : '随机预览'}
          </button>
        </div>
        {preview ? (
          <pre className="taskform__preview" aria-label="item_preview">
            {`#${preview.id}${preview.externalId ? ` · ${preview.externalId}` : ''}\n${JSON.stringify(preview.payload, null, 2)}`}
          </pre>
        ) : (
          <div className="lh-muted lh-text-13" style={{ marginTop: 6 }}>点击「随机预览」查看一条已导入题目。</div>
        )}
      </div>

      <div className="taskform__field">
        <div className="lh-hflex" style={{ justifyContent: 'space-between' }}>
          <label className="taskform__label" style={{ marginBottom: 0 }}>批量编辑题目 payload</label>
          <button type="button" aria-label="加载题目列表" disabled={loadingItems} className="lh-btn lh-btn--sm" onClick={() => void loadItems(true)}>
            {loadingItems ? '加载中…' : '加载题目列表'}
          </button>
        </div>
        {editableItems.length === 0 ? (
          <div className="lh-muted lh-text-13" style={{ marginTop: 6 }}>「加载题目列表」拉取本任务全部题目,编辑后批量覆盖。</div>
        ) : (
          <div className="lh-vflex" style={{ gap: 10, marginTop: 8 }}>
            {editableItems.map((row) => (
              <div key={row.itemId} className="taskform__batch-row">
                <div className="lh-hflex" style={{ justifyContent: 'space-between' }}>
                  <strong className="lh-text-13">
                    #{row.itemId}{row.externalId ? ` · ${row.externalId}` : ''}
                    <span className="lh-muted lh-text-12" style={{ marginLeft: 8 }}>{row.status}</span>
                  </strong>
                  <button type="button" aria-label={`移除批量编辑 ${row.itemId}`} className="lh-btn lh-btn--sm" onClick={() => removeDraft(row.itemId)}>移除</button>
                </div>
                <textarea
                  aria-label={`batch_payload_${row.itemId}`}
                  className="taskform__textarea"
                  value={row.payloadDraft}
                  onChange={(event) => updateDraft(row.itemId, event.target.value)}
                />
              </div>
            ))}
            <div className="lh-hflex" style={{ justifyContent: 'space-between' }}>
              {itemsHasMore ? (
                <button type="button" aria-label="加载更多题目" disabled={loadingItems} className="lh-btn lh-btn--sm" onClick={() => void loadItems(false)}>
                  {loadingItems ? '加载中…' : '加载更多'}
                </button>
              ) : <span />}
              <button type="button" aria-label="保存批量编辑" disabled={savingBatch} className="lh-btn lh-btn--primary" onClick={() => void saveBatch()}>
                {savingBatch ? '保存中…' : '批量保存'}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
