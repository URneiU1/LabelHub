import { showItemModes, widgetTypes, type AnswerValue, type FieldSchema, type ParseResult, type TemplateSchema, type WidgetType } from './types'

const widgetSet = new Set<string>(widgetTypes)
const showItemModeSet = new Set<string>(showItemModes)

export function parseTemplateSchema(raw: string | unknown): ParseResult<TemplateSchema> {
  const parsedResult = parseUnknown(raw)
  if (!parsedResult.ok) {
    return parseError('$', `invalid JSON: ${parsedResult.message}`)
  }
  const parsed = parsedResult.value
  if (!isRecord(parsed)) {
    return parseError('$', 'schema must be an object')
  }
  if (!Array.isArray(parsed.fields)) {
    return parseError('fields', 'fields must be an array')
  }
  if (parsed.fields.length === 0) {
    return parseError('fields', 'fields must be non-empty')
  }
  if (parsed.layout !== undefined && parsed.layout !== 'single_page') {
    return parseError('layout', 'layout must be single_page')
  }

  const fields: FieldSchema[] = []
  const names = new Set<string>()
  for (let index = 0; index < parsed.fields.length; index += 1) {
    const rawField = parsed.fields[index]
    const path = `fields[${index}]`
    if (!isRecord(rawField)) {
      return parseError(path, 'field must be an object')
    }
    const name = stringProp(rawField.name)
    if (!name) {
      return parseError(`${path}.name`, 'name is required')
    }
    if (names.has(name)) {
      return parseError(`${path}.name`, `duplicate name ${name}`)
    }
    names.add(name)

    const widget = stringProp(rawField.widget)
    if (!widgetSet.has(widget)) {
      return parseError(`${path}.widget`, `widget not in enum: ${widget}`)
    }

    const field: FieldSchema = {
      name,
      widget: widget as WidgetType,
      label: stringProp(rawField.label) || name,
    }
    if (typeof rawField.required === 'boolean') {
      field.required = rawField.required
    } else if ('required' in rawField) {
      return parseError(`${path}.required`, 'required must be boolean')
    }
    if (Array.isArray(rawField.options)) {
      field.options = rawField.options.filter((item): item is string | number => typeof item === 'string' || typeof item === 'number')
    }
    const minLength = numberProp(rawField.minLength)
    const maxLength = numberProp(rawField.maxLength)
    if (minLength !== undefined) {
      field.minLength = minLength
    } else if ('minLength' in rawField) {
      return parseError(`${path}.minLength`, 'minLength must be number')
    }
    if (maxLength !== undefined) {
      field.maxLength = maxLength
    } else if ('maxLength' in rawField) {
      return parseError(`${path}.maxLength`, 'maxLength must be number')
    }
    if (field.minLength !== undefined && field.maxLength !== undefined && field.minLength > field.maxLength) {
      return parseError(`${path}.maxLength`, 'minLength cannot exceed maxLength')
    }
    if (typeof rawField.path === 'string') {
      field.path = rawField.path
    }
    if (typeof rawField.mode === 'string') {
      if (!showItemModeSet.has(rawField.mode)) {
        return parseError(`${path}.mode`, `mode not in enum: ${rawField.mode}`)
      }
      field.mode = rawField.mode as FieldSchema['mode']
    }
    const maxFiles = integerProp(rawField.maxFiles)
    if (maxFiles !== undefined && maxFiles > 0) {
      field.maxFiles = maxFiles
    }
    if (typeof rawField.prompt === 'string') {
      field.prompt = rawField.prompt
    }
    if (typeof rawField.target_field === 'string') {
      field.target_field = rawField.target_field
    }
    fields.push(field)
  }

  return {
    ok: true,
    value: {
      version: integerProp(parsed.version),
      title: stringProp(parsed.title) || 'untitled_template',
      layout: 'single_page',
      fields,
    },
  }
}

export function parseAnswer(raw?: string | null): AnswerValue {
  if (!raw) {
    return {}
  }
  try {
    const parsed: unknown = JSON.parse(raw)
    return isRecord(parsed) ? parsed : { value: parsed }
  } catch {
    return { raw }
  }
}

function parseUnknown(raw: string | unknown): { ok: true, value: unknown } | { ok: false, message: string } {
  if (typeof raw !== 'string') {
    return { ok: true, value: raw }
  }
  try {
    return { ok: true, value: JSON.parse(raw) as unknown }
  } catch (error) {
    return { ok: false, message: error instanceof Error ? error.message : 'invalid JSON' }
  }
}

function parseError<T>(field: string, message: string): ParseResult<T> {
  return { ok: false, error: { field, message } }
}

function stringProp(value: unknown) {
  return typeof value === 'string' ? value.trim() : ''
}

function numberProp(value: unknown) {
  if (typeof value !== 'number' || !Number.isInteger(value) || value < 0) {
    return undefined
  }
  return value
}

function integerProp(value: unknown) {
  return typeof value === 'number' && Number.isInteger(value) ? value : undefined
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
