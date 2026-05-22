import type { AnswerValue, TemplateSchema, ValidationError } from './types'

export function validateAnswer(schema: TemplateSchema, answer: AnswerValue): ValidationError[] {
  const errors: ValidationError[] = []
  for (const field of schema.fields) {
    if (field.widget === 'ShowItem') {
      continue
    }
    const value = answer[field.name]
    if (field.required && isEmpty(value)) {
      errors.push({ field: field.name, message: `${field.label} is required` })
      continue
    }
    if (typeof value === 'string') {
      if (field.minLength !== undefined && value.trim().length < field.minLength) {
        errors.push({ field: field.name, message: `${field.label} must be at least ${field.minLength} characters` })
      }
      if (field.maxLength !== undefined && value.length > field.maxLength) {
        errors.push({ field: field.name, message: `${field.label} must be at most ${field.maxLength} characters` })
      }
    }
  }
  return errors
}

function isEmpty(value: unknown) {
  if (value === undefined || value === null) {
    return true
  }
  if (typeof value === 'string') {
    return value.trim() === ''
  }
  if (Array.isArray(value)) {
    return value.length === 0
  }
  return false
}
