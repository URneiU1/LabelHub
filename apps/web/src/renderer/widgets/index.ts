import type { ComponentType } from 'react'
import type { WidgetProps, WidgetType } from '../types'
import InputWidget from './Input'
import PlaceholderWidget from './Placeholder'
import RadioWidget from './Radio'
import ShowItemWidget from './ShowItem'
import TagsWidget from './Tags'
import TextAreaWidget from './TextArea'

export const widgetRegistry: Record<WidgetType, ComponentType<WidgetProps>> = {
  ShowItem: ShowItemWidget,
  Input: InputWidget,
  TextArea: TextAreaWidget,
  Radio: RadioWidget,
  Tags: TagsWidget,
  RichText: PlaceholderWidget,
  JSONEditor: PlaceholderWidget,
  FileUpload: PlaceholderWidget,
  LLMTrigger: PlaceholderWidget,
}
