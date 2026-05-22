package main

import (
	"testing"

	"labelhub-api/internal/model"
)

func TestSeedTemplateActions(t *testing.T) {
	v2ID := uint64(22)
	cases := []struct {
		name     string
		task     model.Task
		existing *model.TaskTemplate
		want     seedTemplateAction
	}{
		{
			name: "existing v1 and task points to v2 preserves active pointer",
			task: model.Task{TemplateID: &v2ID},
			existing: &model.TaskTemplate{
				ID:         11,
				SchemaHash: "same",
			},
			want: seedTemplateAction{CreateTemplate: false, UpdateExisting: false, AttachToTask: false},
		},
		{
			name: "nil task template fills v1 pointer",
			task: model.Task{},
			existing: &model.TaskTemplate{
				ID:         11,
				SchemaHash: "same",
			},
			want: seedTemplateAction{CreateTemplate: false, UpdateExisting: false, AttachToTask: true},
		},
		{
			name: "changed v1 hash updates schema without changing active pointer",
			task: model.Task{TemplateID: &v2ID},
			existing: &model.TaskTemplate{
				ID:         11,
				SchemaHash: "old",
			},
			want: seedTemplateAction{CreateTemplate: false, UpdateExisting: true, AttachToTask: false},
		},
		{
			name:     "missing v1 creates template and attaches when task has no template",
			task:     model.Task{},
			existing: nil,
			want:     seedTemplateAction{CreateTemplate: true, UpdateExisting: false, AttachToTask: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := seedTemplateActions(tc.task, tc.existing, "same")
			if got != tc.want {
				t.Fatalf("seedTemplateActions() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
