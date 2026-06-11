// Package schemadiff 检测标注模板 schema 的版本间结构变更,并按对历史标注/下游消费的
// 影响分级(safe / warning / breaking)。纯函数、无 DB 依赖,供模板校验端点复用。
package schemadiff

import (
	"encoding/json"
	"fmt"
	"sort"
)

// ChangeSeverity 是一次变更对历史标注的破坏程度。
type ChangeSeverity string

const (
	SeveritySafe     ChangeSeverity = "safe"     // 向后兼容,历史标注不受影响
	SeverityWarning  ChangeSeverity = "warning"  // 历史标注可能不再有效/失配,需关注
	SeverityBreaking ChangeSeverity = "breaking" // 历史标注/下游消费会丢数据或类型不兼容
)

// SchemaChange 是一条结构化变更记录。
type SchemaChange struct {
	Field    string         `json:"field"`
	Kind     string         `json:"kind"`
	Severity ChangeSeverity `json:"severity"`
	Detail   string         `json:"detail"`
}

// Summary 是变更的分级计数。
type Summary struct {
	Breaking int `json:"breaking"`
	Warning  int `json:"warning"`
	Safe     int `json:"safe"`
}

// FieldInfo 是一个标注字段在 schema 里参与比较的关键属性。
type FieldInfo struct {
	Name     string
	Widget   string
	Required bool
	Options  []string
	Path     string
}

// containerWidgets 是结构容器(只承载子字段,自身不是标注字段);displayWidgets 只展示、不落答案。
var containerWidgets = map[string]struct{}{"Group": {}, "Tabs": {}}
var displayWidgets = map[string]struct{}{"ShowItem": {}}

// FlattenFields 递归遍历 schema(含 Group.fields 与 Tabs.tabs[].fields 嵌套),产出按字段 name
// 索引的扁平表;容器物料只递归不收录,展示型物料(ShowItem)跳过(不落标注答案)。
// 重复 name 取首次出现(模板校验层已禁止重复)。
func FlattenFields(schemaJSON string) (map[string]FieldInfo, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &doc); err != nil {
		return nil, fmt.Errorf("schemadiff: parse schema: %w", err)
	}
	out := make(map[string]FieldInfo)
	fields, _ := doc["fields"].([]any)
	walkFields("fields", fields, out)
	return out, nil
}

func walkFields(path string, fields []any, out map[string]FieldInfo) {
	for i, raw := range fields {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fieldPath := fmt.Sprintf("%s[%d]", path, i)
		widget, _ := f["widget"].(string)

		if _, isContainer := containerWidgets[widget]; isContainer {
			if children, ok := f["fields"].([]any); ok {
				walkFields(fieldPath+".fields", children, out)
			}
			if tabs, ok := f["tabs"].([]any); ok {
				for j, t := range tabs {
					tab, ok := t.(map[string]any)
					if !ok {
						continue
					}
					if children, ok := tab["fields"].([]any); ok {
						walkFields(fmt.Sprintf("%s.tabs[%d].fields", fieldPath, j), children, out)
					}
				}
			}
			continue
		}
		if _, isDisplay := displayWidgets[widget]; isDisplay {
			continue
		}

		name, _ := f["name"].(string)
		if name == "" {
			continue
		}
		if _, exists := out[name]; exists {
			continue
		}
		out[name] = FieldInfo{
			Name:     name,
			Widget:   widget,
			Required: boolProp(f, "required"),
			Options:  stringSliceProp(f, "options"),
			Path:     fieldPath,
		}
	}
}

// DetectChanges 比对旧/新 schema,返回按严重度(breaking→warning→safe)、再按字段名排序的变更列表。
func DetectChanges(oldJSON, newJSON string) ([]SchemaChange, error) {
	oldFields, err := FlattenFields(oldJSON)
	if err != nil {
		return nil, err
	}
	newFields, err := FlattenFields(newJSON)
	if err != nil {
		return nil, err
	}

	var changes []SchemaChange
	for name, of := range oldFields {
		nf, ok := newFields[name]
		if !ok {
			changes = append(changes, SchemaChange{name, "field_removed", SeverityBreaking,
				fmt.Sprintf("字段 %q 被删除;依赖该字段的历史标注与下游消费将丢失数据", name)})
			continue
		}
		if of.Widget != nf.Widget {
			changes = append(changes, SchemaChange{name, "widget_changed", SeverityBreaking,
				fmt.Sprintf("字段 %q 控件由 %s 改为 %s;历史标注取值类型可能不再兼容", name, of.Widget, nf.Widget)})
		}
		if !of.Required && nf.Required {
			changes = append(changes, SchemaChange{name, "required_added", SeverityWarning,
				fmt.Sprintf("字段 %q 由可选改为必填;缺少该字段的历史标注将不再有效", name)})
		}
		if removed := removedOptions(of.Options, nf.Options); len(removed) > 0 {
			changes = append(changes, SchemaChange{name, "options_removed", SeverityWarning,
				fmt.Sprintf("字段 %q 移除选项 %v;选中了已删除项的历史标注将失配", name, removed)})
		}
	}
	for name, nf := range newFields {
		if _, ok := oldFields[name]; ok {
			continue
		}
		if nf.Required {
			changes = append(changes, SchemaChange{name, "required_field_added", SeverityWarning,
				fmt.Sprintf("新增必填字段 %q;历史标注缺少该字段", name)})
		} else {
			changes = append(changes, SchemaChange{name, "field_added", SeveritySafe,
				fmt.Sprintf("新增可选字段 %q;向后兼容", name)})
		}
	}

	sort.SliceStable(changes, func(i, j int) bool {
		ri, rj := severityRank(changes[i].Severity), severityRank(changes[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if changes[i].Field != changes[j].Field {
			return changes[i].Field < changes[j].Field
		}
		return changes[i].Kind < changes[j].Kind
	})
	return changes, nil
}

// Summarize 统计各严重度的变更条数。
func Summarize(changes []SchemaChange) Summary {
	var s Summary
	for _, c := range changes {
		switch c.Severity {
		case SeverityBreaking:
			s.Breaking++
		case SeverityWarning:
			s.Warning++
		default:
			s.Safe++
		}
	}
	return s
}

func severityRank(s ChangeSeverity) int {
	switch s {
	case SeverityBreaking:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

func boolProp(f map[string]any, key string) bool {
	b, _ := f[key].(bool)
	return b
}

// stringSliceProp 取一个 options 数组并统一转字符串(选项可为字符串或数字,如评分档位);
// 与 handler.stringSliceProp(只收字符串)语义不同,此处刻意把数字选项也纳入比较。
func stringSliceProp(f map[string]any, key string) []string {
	raw, ok := f[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		switch t := v.(type) {
		case string:
			out = append(out, t)
		case json.Number:
			out = append(out, t.String())
		case float64:
			out = append(out, fmt.Sprintf("%v", t))
		}
	}
	return out
}

// removedOptions 返回 old 中存在、new 中不再存在的选项(保持 old 顺序)。
func removedOptions(oldOpts, newOpts []string) []string {
	if len(oldOpts) == 0 {
		return nil
	}
	present := make(map[string]struct{}, len(newOpts))
	for _, o := range newOpts {
		present[o] = struct{}{}
	}
	var removed []string
	for _, o := range oldOpts {
		if _, ok := present[o]; !ok {
			removed = append(removed, o)
		}
	}
	return removed
}
