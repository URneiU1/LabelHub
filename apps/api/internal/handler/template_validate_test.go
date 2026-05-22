package handler

import (
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
			name: "malformed JSON",
			raw:  `{not json`,
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
			if !contains(errs[0].Message, tc.wantMsg) {
				t.Errorf("Message %q does not contain %q", errs[0].Message, tc.wantMsg)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
