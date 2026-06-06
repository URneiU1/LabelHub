package handler

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestValidateTemplateSchema(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantValid bool
		wantField string // first error's field, "" if none
		wantMsg   string // substring of first error's message
	}{
		{
			name: "happy minimal",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","label":"A","required":false}
			]}`,
			wantValid: true,
		},
		{
			name:      "fields empty",
			raw:       `{"title":"t","layout":"single_page","fields":[]}`,
			wantValid: false, wantField: "fields", wantMsg: "non-empty",
		},
		{
			name: "duplicate name",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input"},
				{"name":"a","widget":"Radio"}
			]}`,
			wantValid: false, wantField: "fields[1].name", wantMsg: "duplicate",
		},
		{
			name: "duplicate name after trim",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"score","widget":"Input"},
				{"name":" score ","widget":"TextArea"}
			]}`,
			wantValid: false, wantField: "fields[1].name", wantMsg: "duplicate",
		},
		{
			name: "reserved field name",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"value","widget":"Input"}
			]}`,
			wantValid: false, wantField: "fields[0].name", wantMsg: "reserved",
		},
		{
			name: "unknown widget",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Slider"}
			]}`,
			wantValid: false, wantField: "fields[0].widget", wantMsg: "widget not in enum",
		},
		{
			name: "required non-bool via JSON string",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","required":"yes"}
			]}`,
			wantValid: false, wantField: "fields[0].required", wantMsg: "must be bool",
		},
		{
			name: "min > max",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","minLength":10,"maxLength":5}
			]}`,
			wantValid: false, wantField: "fields[0].maxLength", wantMsg: "min > max",
		},
		{
			name: "negative minLength",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","minLength":-5}
			]}`,
			wantValid: false, wantField: "fields[0].minLength", wantMsg: ">= 0",
		},
		{
			name: "minLength non-number",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","minLength":"5"}
			]}`,
			wantValid: false, wantField: "fields[0].minLength", wantMsg: "must be number",
		},
		{
			name: "minLength fractional",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"a","widget":"Input","minLength":1.5}
			]}`,
			wantValid: false, wantField: "fields[0].minLength", wantMsg: "must be number",
		},
		{
			name: "radio requires options",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"score","widget":"Radio"}
			]}`,
			wantValid: false, wantField: "fields[0].options", wantMsg: "non-empty",
		},
		{
			name: "option object rejected",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"score","widget":"Radio","options":[{"label":"A","value":"a"}]}
			]}`,
			wantValid: false, wantField: "fields[0].options[0]", wantMsg: "string or number",
		},
		{
			name: "file upload maxFiles must be positive",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"evidence","widget":"FileUpload","maxFiles":0}
			]}`,
			wantValid: false, wantField: "fields[0].maxFiles", wantMsg: "> 0",
		},
		{
			name: "llm trigger target must exist",
			raw: `{"title":"t","layout":"single_page","fields":[
				{"name":"ai","widget":"LLMTrigger","target_field":"missing"}
			]}`,
			wantValid: false, wantField: "fields[0].target_field", wantMsg: "existing field",
		},
		{
			name: "llm trigger external target must be explicit",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"ai","widget":"LLMTrigger","target_field":"external.score","x-allow-external-target":true}
				]}`,
			wantValid: true,
		},
		{
			name: "regex and requiredWhen valid",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"decision","widget":"Radio","options":["pass","reject"]},
					{"name":"reason","widget":"Input","regex":"^.{4,}$","requiredWhen":{"field":"decision","equals":"reject"}}
				]}`,
			wantValid: true,
		},
		{
			name: "invalid regex rejected",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"summary","widget":"Input","regex":"["}
				]}`,
			wantValid: false, wantField: "fields[0].regex", wantMsg: "valid",
		},
		{
			name: "requiredWhen dangling field rejected",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"reason","widget":"Input","requiredWhen":{"field":"missing","notEmpty":true}}
				]}`,
			wantValid: false, wantField: "fields[0].requiredWhen.field", wantMsg: "existing field",
		},
		{
			name: "requiredWhen needs condition",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"reason","widget":"Input","requiredWhen":{"field":"reason"}}
				]}`,
			wantValid: false, wantField: "fields[0].requiredWhen", wantMsg: "equals or notEmpty",
		},
		{
			name: "visibleWhen and customRule valid",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"decision","widget":"Radio","options":["pass","reject"]},
					{"name":"reason","widget":"Input","visibleWhen":{"field":"decision","equals":"reject"}},
					{"name":"score","widget":"Input","customRule":{"expr":"len(value) >= 4","message":"至少 4 个字符"}}
				]}`,
			wantValid: true,
		},
		{
			name: "visibleWhen dangling field rejected",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"reason","widget":"Input","visibleWhen":{"field":"missing","notEmpty":true}}
				]}`,
			wantValid: false, wantField: "fields[0].visibleWhen.field", wantMsg: "existing field",
		},
		{
			name: "visibleWhen needs condition",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"reason","widget":"Input","visibleWhen":{"field":"reason"}}
				]}`,
			wantValid: false, wantField: "fields[0].visibleWhen", wantMsg: "equals or notEmpty",
		},
		{
			name: "customRule must be object",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"score","widget":"Input","customRule":"value > 0"}
				]}`,
			wantValid: false, wantField: "fields[0].customRule", wantMsg: "object",
		},
		{
			name: "customRule empty expr",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"score","widget":"Input","customRule":{"expr":"","message":"invalid"}}
				]}`,
			wantValid: false, wantField: "fields[0].customRule.expr", wantMsg: "required",
		},
		{
			name: "customRule invalid expr",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"score","widget":"Input","customRule":{"expr":"value >= ","message":"invalid"}}
				]}`,
			wantValid: false, wantField: "fields[0].customRule.expr", wantMsg: "unsupported syntax",
		},
		{
			name: "customRule unsupported function rejected at save",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"score","widget":"Input","customRule":{"expr":"max(value, 1) > 0","message":"invalid"}}
				]}`,
			wantValid: false, wantField: "fields[0].customRule.expr", wantMsg: "unsupported",
		},
		{
			name: "customRule unsupported operator rejected at save",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"score","widget":"Input","customRule":{"expr":"value > 0 && value < 9","message":"invalid"}}
				]}`,
			wantValid: false, wantField: "fields[0].customRule.expr", wantMsg: "unsupported",
		},
		{
			name: "customRule empty message",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"score","widget":"Input","customRule":{"expr":"value > 0","message":""}}
				]}`,
			wantValid: false, wantField: "fields[0].customRule.message", wantMsg: "required",
		},
		{
			name: "group and tabs nested fields are valid",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"summary","widget":"Input"},
					{"name":"group_1","widget":"Group","fields":[
						{"name":"group_summary","widget":"TextArea"},
						{"name":"group_decision","widget":"Radio","options":["pass","reject"]}
					]},
					{"name":"tabs_1","widget":"Tabs","tabs":[
						{"label":"基础","fields":[{"name":"tabs_score","widget":"Input","minLength":1,"maxLength":10}]},
						{"label":"复核","fields":[{"name":"tabs_ai","widget":"LLMTrigger","target_field":"group_summary"}]}
					]}
				]}`,
			wantValid: true,
		},
		{
			name: "nested duplicate name",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"summary","widget":"Input"},
					{"name":"group_1","widget":"Group","fields":[{"name":"summary","widget":"TextArea"}]}
				]}`,
			wantValid: false, wantField: "fields[1].fields[0].name", wantMsg: "duplicate",
		},
		{
			name: "tabs require fields",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"tabs_1","widget":"Tabs","tabs":[{"label":"Empty","fields":[]}]}
				]}`,
			wantValid: false, wantField: "fields[0].tabs[0].fields", wantMsg: "non-empty",
		},
		{
			name: "nested llm trigger target must exist",
			raw: `{"title":"t","layout":"single_page","fields":[
					{"name":"group_1","widget":"Group","fields":[{"name":"ai","widget":"LLMTrigger","target_field":"missing"}]}
				]}`,
			wantValid: false, wantField: "fields[0].fields[0].target_field", wantMsg: "existing field",
		},
		{
			name:      "malformed JSON",
			raw:       `{not json`,
			wantValid: false, wantField: "$", wantMsg: "invalid JSON",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateTemplateSchema(tc.raw)
			if tc.wantValid {
				if len(errs) != 0 {
					t.Fatalf("expected valid, got errors: %+v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatal("expected errors, got none")
			}
			if errs[0].Field != tc.wantField {
				t.Errorf("Field = %q, want %q", errs[0].Field, tc.wantField)
			}
			if !strings.Contains(errs[0].Message, tc.wantMsg) {
				t.Errorf("Message %q does not contain %q", errs[0].Message, tc.wantMsg)
			}
		})
	}
}

func TestValidateTemplateSchemaLimits(t *testing.T) {
	fields := make([]map[string]any, maxTemplateFields+1)
	for i := range fields {
		fields[i] = map[string]any{"name": "f", "widget": "Input"}
	}
	raw, _ := json.Marshal(map[string]any{"fields": fields})
	errs := validateTemplateSchema(string(raw))
	if len(errs) == 0 || errs[0].Field != "fields" {
		t.Fatalf("expected fields limit error, got %+v", errs)
	}

	fields = []map[string]any{{
		"name":   "group",
		"widget": "Group",
		"fields": make([]map[string]any, maxTemplateFields),
	}}
	for i := range fields[0]["fields"].([]map[string]any) {
		fields[0]["fields"].([]map[string]any)[i] = map[string]any{"name": fmt.Sprintf("nested_%d", i), "widget": "Input"}
	}
	raw, _ = json.Marshal(map[string]any{"fields": fields})
	errs = validateTemplateSchema(string(raw))
	if len(errs) == 0 || errs[0].Field != "fields" {
		t.Fatalf("expected recursive fields limit error, got %+v", errs)
	}

	options := make([]any, maxTemplateOptions+1)
	for i := range options {
		options[i] = map[string]any{"label": "A", "value": i}
	}
	raw, _ = json.Marshal(map[string]any{"fields": []map[string]any{{
		"name": "choice", "widget": "Radio", "options": options,
	}}})
	errs = validateTemplateSchema(string(raw))
	if len(errs) == 0 || errs[0].Field != "fields[0].options" {
		t.Fatalf("expected options limit error, got %+v", errs)
	}
}
