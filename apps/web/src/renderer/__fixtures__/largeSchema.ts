// buildLargeSchema 生成一个确定性的大 schema(默认 ~300 个叶子字段),覆盖
// Input/TextArea/Tags + Radio 开关 + visibleWhen 依赖 + Group + Tabs,用于
// 大表单的性能与健壮性冒烟(渲染不崩、只提交可见字段、解析不损坏字段顺序/键)。
//
// 完全确定性(无随机),便于断言与跨次复现。

type RawField = Record<string, unknown>
type RawSchema = { title: string; fields: RawField[] }

const leafWidgets = ['Input', 'TextArea', 'Tags']

function leafField(index: number, extra: RawField = {}): RawField {
  const widget = leafWidgets[index % leafWidgets.length]
  const base: RawField = { name: `f_${index}`, widget, label: `字段 ${index}` }
  if (widget === 'Tags') {
    base.options = ['a', 'b', 'c']
  }
  return { ...base, ...extra }
}

export function buildLargeSchema(leafCount = 300): RawSchema {
  const fields: RawField[] = []
  let made = 0
  let index = 0

  // 70% 平铺区:每 10 个插入一个 Radio 开关 + 一个 visibleWhen 依赖字段。
  const flatTarget = Math.floor(leafCount * 0.7)
  while (made < flatTarget) {
    if (index % 10 === 0) {
      fields.push({ name: `gate_${index}`, widget: 'Radio', label: `开关 ${index}`, options: ['show', 'hide'] })
      made++
      fields.push(leafField(index + 1, { visibleWhen: { field: `gate_${index}`, equals: 'show' } }))
      made++
      index += 2
    } else {
      fields.push(leafField(index))
      made++
      index++
    }
  }

  // 15% 放进一个 Group。
  const groupTarget = Math.floor(leafCount * 0.15)
  const groupFields: RawField[] = []
  for (let g = 0; g < groupTarget; g++) {
    groupFields.push(leafField(index))
    index++
    made++
  }
  fields.push({ name: 'group_main', widget: 'Group', label: '分组', fields: groupFields })

  // 余下放进一个两页 Tabs。
  const remaining = Math.max(2, leafCount - made)
  const perTab = Math.ceil(remaining / 2)
  const tabA: RawField[] = []
  const tabB: RawField[] = []
  for (let t = 0; t < remaining; t++) {
    ;(t < perTab ? tabA : tabB).push(leafField(index))
    index++
    made++
  }
  fields.push({
    name: 'tabs_main',
    widget: 'Tabs',
    label: '分页',
    tabs: [
      { label: 'A', fields: tabA },
      { label: 'B', fields: tabB.length > 0 ? tabB : [leafField(index)] },
    ],
  })

  return { title: '大表单', fields }
}

// flatLeafNames 返回 schema 顶层字段的 name 顺序(用于断言解析后顺序不被打乱)。
export function topLevelFieldNames(schema: RawSchema): string[] {
  return schema.fields.map((field) => String(field.name))
}
