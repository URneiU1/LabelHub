import type { ChangeEvent, CSSProperties } from 'react'
import { useState } from 'react'
import { apiUpload } from '../../shared/api/client'
import type { WidgetProps } from '../types'
import FieldFrame from './FieldFrame'

type UploadedFile = {
  id: number
  storageKey: string
  originalName: string
}

export default function FileUploadWidget({ field, value, runtime, readOnly, onChange }: WidgetProps) {
  const [uploading, setUploading] = useState(false)
  const [status, setStatus] = useState('')
  const files = Array.isArray(value) ? value.map(String) : []
  const maxFiles = field.maxFiles ?? 5
  const disabled = readOnly || uploading || !runtime?.taskId || files.length >= maxFiles

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    if (!file || !runtime?.taskId) {
      event.target.value = ''
      return
    }
    setUploading(true)
    try {
      const form = new FormData()
      form.append('task_id', String(runtime.taskId))
      form.append('file', file)
      const uploaded = await apiUpload<UploadedFile>('/uploads', form)
      onChange(field.name, [...files, uploaded.storageKey])
      setStatus(`已上传 ${uploaded.originalName}`)
    } catch (error) {
      setStatus(error instanceof Error ? error.message : '上传失败')
    } finally {
      event.target.value = ''
      setUploading(false)
    }
  }

  function remove(storageKey: string) {
    onChange(field.name, files.filter((item) => item !== storageKey))
  }

  return (
    <FieldFrame label={field.label} required={field.required}>
      <input
        aria-label={field.label}
        type="file"
        disabled={disabled}
        onChange={(event) => void upload(event)}
      />
      {status ? <span style={statusStyle}>{status}</span> : null}
      {files.length > 0 ? (
        <ul style={fileListStyle}>
          {files.map((file) => (
            <li key={file} style={fileItemStyle}>
              <code style={fileKeyStyle}>{file}</code>
              {readOnly ? null : (
                <button type="button" style={linkButtonStyle} onClick={() => remove(file)}>
                  删除
                </button>
              )}
            </li>
          ))}
        </ul>
      ) : null}
    </FieldFrame>
  )
}

const fileListStyle: CSSProperties = {
  display: 'grid',
  gap: 'var(--space-xs)',
  margin: 'var(--space-sm) 0 0',
  padding: 0,
  listStyle: 'none',
}

const fileItemStyle: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 'var(--space-sm)',
  padding: 'var(--space-xs) var(--space-sm)',
  border: '1px solid var(--color-border-light)',
  background: 'var(--color-bg)',
}

const fileKeyStyle: CSSProperties = {
  minWidth: 0,
  overflowWrap: 'anywhere',
}

const linkButtonStyle: CSSProperties = {
  border: 0,
  background: 'transparent',
  color: 'var(--color-accent)',
  cursor: 'pointer',
}

const statusStyle: CSSProperties = {
  color: 'var(--color-text-secondary)',
  fontSize: 'var(--text-sm)',
}
