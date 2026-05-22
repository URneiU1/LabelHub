export const widgetTypes = [
  'ShowItem',
  'Input',
  'TextArea',
  'Radio',
  'Tags',
  'RichText',
  'JSONEditor',
  'FileUpload',
  'LLMTrigger',
] as const

export type WidgetType = typeof widgetTypes[number]

export const showItemModes = ['auto', 'text', 'video', 'image', 'markdown', 'json'] as const

export type ShowItemMode = typeof showItemModes[number]

export type FieldOption = string | number

export type FieldSchema = {
  name: string
  widget: WidgetType
  label: string
  required?: boolean
  options?: FieldOption[]
  minLength?: number
  maxLength?: number
  path?: string
  mode?: ShowItemMode
  maxFiles?: number
  prompt?: string
  target_field?: string
}

export type TemplateSchema = {
  version?: number
  title: string
  layout: 'single_page'
  fields: FieldSchema[]
}

export type AnswerValue = Record<string, unknown>

export type RenderPayload = Record<string, unknown>

export type SchemaParseError = {
  field: string
  message: string
}

export type ParseResult<T> =
  | { ok: true, value: T }
  | { ok: false, error: SchemaParseError }

export type ValidationError = {
  field: string
  message: string
}

export type WidgetProps = {
  field: FieldSchema
  value: unknown
  answer: AnswerValue
  payload: RenderPayload
  readOnly?: boolean
  onChange: (name: string, value: unknown) => void
}
