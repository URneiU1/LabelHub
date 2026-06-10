import type { ComponentType } from 'react'
import type { WidgetProps, WidgetType } from '../types'
import FileUploadWidget from './FileUpload'
import GroupWidget from './Group'
import InputWidget from './Input'
import JSONEditorWidget from './JSONEditor'
import LLMTriggerWidget from './LLMTrigger'
import RadioWidget from './Radio'
import RichTextWidget from './RichText'
import ShowItemWidget from './ShowItem'
import TabsWidget from './Tabs'
import TagsWidget from './Tags'
import TextAreaWidget from './TextArea'

export const widgetRegistry: Record<WidgetType, ComponentType<WidgetProps>> = {
  ShowItem: ShowItemWidget,
  Group: GroupWidget,
  Tabs: TabsWidget,
  Input: InputWidget,
  TextArea: TextAreaWidget,
  Radio: RadioWidget,
  MultiSelect: TagsWidget,
  Tags: TagsWidget,
  RichText: RichTextWidget,
  JSONEditor: JSONEditorWidget,
  FileUpload: FileUploadWidget,
  ImageUpload: FileUploadWidget,
  LLMTrigger: LLMTriggerWidget,
}
