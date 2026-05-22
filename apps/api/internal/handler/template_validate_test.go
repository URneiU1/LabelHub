package handler

import (
	"encoding/json"
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
