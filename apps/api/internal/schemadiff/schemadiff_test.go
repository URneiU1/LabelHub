package schemadiff

import "testing"

// schema 构造：单层 fields。
func schemaWith(fields string) string {
	return `{"title":"t","layout":"single_page","fields":[` + fields + `]}`
}

func changeByField(changes []SchemaChange, field string) (SchemaChange, bool) {
	for _, c := range changes {
		if c.Field == field {
			return c, true
		}
	}
	return SchemaChange{}, false
}

func TestFlattenFields_RecursesGroupAndTabsSkipsDisplay(t *testing.T) {
	schema := `{"fields":[
		{"name":"panel","widget":"Group","fields":[
			{"name":"prompt_show","widget":"ShowItem"},
			{"name":"answer_text","widget":"Input","required":true}
		]},
		{"name":"tabs","widget":"Tabs","tabs":[
			{"label":"a","fields":[{"name":"verdict","widget":"Radio","options":["A","B"]}]}
		]}
	]}`
	fields, err := FlattenFields(schema)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	// Group/Tabs 容器不收录,ShowItem 跳过,叶子数据字段收录。
	if len(fields) != 2 {
		t.Fatalf("expected 2 data fields, got %d: %v", len(fields), fields)
	}
	if _, ok := fields["panel"]; ok {
		t.Fatal("container Group must not be a field")
	}
	if _, ok := fields["prompt_show"]; ok {
		t.Fatal("display ShowItem must be skipped")
	}
	if f := fields["answer_text"]; !f.Required || f.Widget != "Input" {
		t.Fatalf("answer_text = %+v", f)
	}
	if f := fields["verdict"]; len(f.Options) != 2 {
		t.Fatalf("verdict options = %v", f.Options)
	}
}

func TestDetectChanges_FieldRemovedIsBreaking(t *testing.T) {
	old := schemaWith(`{"name":"a","widget":"Input"},{"name":"b","widget":"Input"}`)
	next := schemaWith(`{"name":"a","widget":"Input"}`)
	changes, err := DetectChanges(old, next)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	c, ok := changeByField(changes, "b")
	if !ok || c.Kind != "field_removed" || c.Severity != SeverityBreaking {
		t.Fatalf("expected breaking field_removed for b, got %+v", changes)
	}
}

func TestDetectChanges_WidgetChangeIsBreaking(t *testing.T) {
	old := schemaWith(`{"name":"a","widget":"Input"}`)
	next := schemaWith(`{"name":"a","widget":"TextArea"}`)
	changes, _ := DetectChanges(old, next)
	c, ok := changeByField(changes, "a")
	if !ok || c.Kind != "widget_changed" || c.Severity != SeverityBreaking {
		t.Fatalf("expected breaking widget_changed, got %+v", changes)
	}
}

func TestDetectChanges_RequiredAddedIsWarning(t *testing.T) {
	old := schemaWith(`{"name":"a","widget":"Input"}`)
	next := schemaWith(`{"name":"a","widget":"Input","required":true}`)
	changes, _ := DetectChanges(old, next)
	c, ok := changeByField(changes, "a")
	if !ok || c.Kind != "required_added" || c.Severity != SeverityWarning {
		t.Fatalf("expected warning required_added, got %+v", changes)
	}
}

func TestDetectChanges_OptionsRemovedIsWarning(t *testing.T) {
	old := schemaWith(`{"name":"v","widget":"Radio","options":["A","B","tie"]}`)
	next := schemaWith(`{"name":"v","widget":"Radio","options":["A","B"]}`)
	changes, _ := DetectChanges(old, next)
	c, ok := changeByField(changes, "v")
	if !ok || c.Kind != "options_removed" || c.Severity != SeverityWarning {
		t.Fatalf("expected warning options_removed, got %+v", changes)
	}
}

func TestDetectChanges_AddedRequiredWarningAddedOptionalSafe(t *testing.T) {
	old := schemaWith(`{"name":"a","widget":"Input"}`)
	next := schemaWith(`{"name":"a","widget":"Input"},{"name":"req","widget":"Input","required":true},{"name":"opt","widget":"Input"}`)
	changes, _ := DetectChanges(old, next)
	if c, ok := changeByField(changes, "req"); !ok || c.Severity != SeverityWarning || c.Kind != "required_field_added" {
		t.Fatalf("req should be warning required_field_added, got %+v", c)
	}
	if c, ok := changeByField(changes, "opt"); !ok || c.Severity != SeveritySafe || c.Kind != "field_added" {
		t.Fatalf("opt should be safe field_added, got %+v", c)
	}
}

func TestDetectChanges_OptionsAddedAndRequiredRelaxedAreSafeNoChange(t *testing.T) {
	old := schemaWith(`{"name":"v","widget":"Radio","options":["A"],"required":true}`)
	next := schemaWith(`{"name":"v","widget":"Radio","options":["A","B"]}`)
	changes, _ := DetectChanges(old, next)
	// 新增选项 + 必填→可选 都是向后兼容,不应产生任何变更记录。
	if len(changes) != 0 {
		t.Fatalf("expected no changes for option-add + required-relax, got %+v", changes)
	}
}

func TestDetectChanges_IdenticalSchemaYieldsNoChanges(t *testing.T) {
	s := schemaWith(`{"name":"a","widget":"Input","required":true},{"name":"b","widget":"Radio","options":["x","y"]}`)
	changes, err := DetectChanges(s, s)
	if err != nil || len(changes) != 0 {
		t.Fatalf("identical schema should have no changes, got %d err=%v", len(changes), err)
	}
}

func TestDetectChanges_SortedBreakingFirstAndSummary(t *testing.T) {
	old := schemaWith(`{"name":"keep","widget":"Input"},{"name":"gone","widget":"Input"},{"name":"opt_field","widget":"Radio","options":["A","B"]}`)
	next := schemaWith(`{"name":"keep","widget":"Input"},{"name":"opt_field","widget":"Radio","options":["A"]},{"name":"added","widget":"Input"}`)
	changes, _ := DetectChanges(old, next)
	if len(changes) < 3 {
		t.Fatalf("expected >=3 changes, got %+v", changes)
	}
	// 排序:breaking 必须排在最前。
	if changes[0].Severity != SeverityBreaking {
		t.Fatalf("breaking change must sort first, got %+v", changes)
	}
	sum := Summarize(changes)
	if sum.Breaking != 1 || sum.Warning != 1 || sum.Safe != 1 {
		t.Fatalf("summary = %+v (changes=%+v)", sum, changes)
	}
}

func TestDetectChanges_InvalidJSONErrors(t *testing.T) {
	valid := schemaWith(`{"name":"a","widget":"Input"}`)
	if _, err := DetectChanges("{bad", valid); err == nil {
		t.Fatal("expected error for invalid old schema json")
	}
	if _, err := DetectChanges(valid, "{bad"); err == nil {
		t.Fatal("expected error for invalid new schema json")
	}
}

func TestFlattenFields_DeepNestingTabsThenGroup(t *testing.T) {
	// Tabs → Group → Input 三层嵌套:递归必须穿透到最内层数据字段。
	schema := `{"fields":[
		{"name":"tabs","widget":"Tabs","tabs":[
			{"label":"t1","fields":[
				{"name":"grp","widget":"Group","fields":[
					{"name":"deep_field","widget":"TextArea","required":true}
				]}
			]}
		]}
	]}`
	fields, err := FlattenFields(schema)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	f, ok := fields["deep_field"]
	if !ok || f.Widget != "TextArea" || !f.Required {
		t.Fatalf("deep nested field not flattened correctly: %+v", fields)
	}
	if len(fields) != 1 {
		t.Fatalf("only the leaf data field should be collected, got %d: %v", len(fields), fields)
	}
}
