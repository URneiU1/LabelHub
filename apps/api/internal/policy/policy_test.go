package policy

import (
	"testing"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/model"
)

func ptrU64(v uint64) *uint64 { return &v }

func TestHasRole(t *testing.T) {
	cases := []struct {
		name   string
		claims *auth.Claims
		role   string
		want   bool
	}{
		{"nil claims", nil, "admin", false},
		{"empty roles", &auth.Claims{Roles: nil}, "admin", false},
		{"role present", &auth.Claims{Roles: []string{"owner", "reviewer"}}, "reviewer", true},
		{"role absent", &auth.Claims{Roles: []string{"owner"}}, "admin", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasRole(tc.claims, tc.role); got != tc.want {
				t.Errorf("HasRole = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsTaskOwner(t *testing.T) {
	task := model.Task{ID: 1, OwnerID: 10}
	cases := []struct {
		name   string
		claims *auth.Claims
		want   bool
	}{
		{"admin always", &auth.Claims{UserID: 99, Roles: []string{"admin"}}, true},
		{"owner match", &auth.Claims{UserID: 10, Roles: []string{"owner"}}, true},
		{"owner mismatch", &auth.Claims{UserID: 11, Roles: []string{"owner"}}, false},
		{"labeler never", &auth.Claims{UserID: 10, Roles: []string{"labeler"}}, false},
		{"no role", &auth.Claims{UserID: 10, Roles: []string{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTaskOwner(tc.claims, task); got != tc.want {
				t.Errorf("IsTaskOwner = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanReadTask(t *testing.T) {
	cases := []struct {
		name   string
		claims *auth.Claims
		task   model.Task
		want   bool
	}{
		{"admin sees draft", &auth.Claims{UserID: 1, Roles: []string{"admin"}}, model.Task{OwnerID: 99, Status: "draft"}, true},
		{"owner sees own draft", &auth.Claims{UserID: 10, Roles: []string{"owner"}}, model.Task{OwnerID: 10, Status: "draft"}, true},
		{"owner blocked from other draft", &auth.Claims{UserID: 11, Roles: []string{"owner"}}, model.Task{OwnerID: 10, Status: "draft"}, false},
		{"labeler sees published", &auth.Claims{UserID: 7, Roles: []string{"labeler"}}, model.Task{OwnerID: 10, Status: "published"}, true},
		{"labeler blocked from draft", &auth.Claims{UserID: 7, Roles: []string{"labeler"}}, model.Task{OwnerID: 10, Status: "draft"}, false},
		{"labeler blocked from paused", &auth.Claims{UserID: 7, Roles: []string{"labeler"}}, model.Task{OwnerID: 10, Status: "paused"}, false},
		{"reviewer sees published", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, model.Task{OwnerID: 10, Status: "published"}, true},
		{"reviewer blocked from archived", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, model.Task{OwnerID: 10, Status: "archived"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanReadTask(tc.claims, tc.task); got != tc.want {
				t.Errorf("CanReadTask = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanClaimNew(t *testing.T) {
	labeler := &auth.Claims{UserID: 7, Roles: []string{"labeler"}}
	cases := []struct {
		name           string
		claims         *auth.Claims
		task           model.Task
		wantAllowed    bool
		wantReason     ClaimDenyReason
		wantTaskStatus string
	}{
		{"labeler on published OK", labeler, model.Task{Status: "published"}, true, "", ""},
		{"labeler on draft → task_not_published", labeler, model.Task{Status: "draft"}, false, ClaimDenyTaskNotPublished, "draft"},
		{"labeler on paused → task_not_published", labeler, model.Task{Status: "paused"}, false, ClaimDenyTaskNotPublished, "paused"},
		{"labeler on archived → task_not_published", labeler, model.Task{Status: "archived"}, false, ClaimDenyTaskNotPublished, "archived"},
		{"owner cannot claim", &auth.Claims{UserID: 10, Roles: []string{"owner"}}, model.Task{Status: "published"}, false, ClaimDenyNotLabeler, ""},
		{"reviewer cannot claim", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, model.Task{Status: "published"}, false, ClaimDenyNotLabeler, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CanClaimNew(tc.claims, tc.task)
			if got.Allowed != tc.wantAllowed {
				t.Errorf("Allowed = %v, want %v", got.Allowed, tc.wantAllowed)
			}
			if !tc.wantAllowed {
				if got.Reason != tc.wantReason {
					t.Errorf("Reason = %q, want %q", got.Reason, tc.wantReason)
				}
				if got.TaskStatus != tc.wantTaskStatus {
					t.Errorf("TaskStatus = %q, want %q", got.TaskStatus, tc.wantTaskStatus)
				}
			}
		})
	}
}

func TestCanReadItem(t *testing.T) {
	task := model.Task{ID: 1, OwnerID: 10, Status: "published"}
	itemUnclaimed := model.TaskItem{ID: 100, TaskID: 1, ClaimedBy: nil}
	itemClaimedBy7 := model.TaskItem{ID: 101, TaskID: 1, ClaimedBy: ptrU64(7)}
	itemClaimedBy8 := model.TaskItem{ID: 102, TaskID: 1, ClaimedBy: ptrU64(8)}
	subHumanReviewing := &model.Submission{ID: 1, ItemID: 101, Status: "human_reviewing"}
	subApproved := &model.Submission{ID: 2, ItemID: 102, Status: "approved"}
	subDraft := &model.Submission{ID: 3, ItemID: 103, Status: "draft"}
	subAIReviewing := &model.Submission{ID: 4, ItemID: 104, Status: "ai_reviewing"}

	cases := []struct {
		name       string
		claims     *auth.Claims
		item       model.TaskItem
		submission *model.Submission
		want       bool
	}{
		{"admin sees any item", &auth.Claims{UserID: 99, Roles: []string{"admin"}}, itemUnclaimed, nil, true},
		{"owner sees own task item", &auth.Claims{UserID: 10, Roles: []string{"owner"}}, itemUnclaimed, nil, true},
		{"other owner blocked", &auth.Claims{UserID: 11, Roles: []string{"owner"}}, itemUnclaimed, nil, false},
		{"labeler sees own claimed", &auth.Claims{UserID: 7, Roles: []string{"labeler"}}, itemClaimedBy7, nil, true},
		{"labeler blocked from unclaimed", &auth.Claims{UserID: 7, Roles: []string{"labeler"}}, itemUnclaimed, nil, false},
		{"labeler blocked from peer's claim", &auth.Claims{UserID: 7, Roles: []string{"labeler"}}, itemClaimedBy8, nil, false},
		{"reviewer sees human_reviewing", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, itemClaimedBy7, subHumanReviewing, true},
		{"reviewer sees approved", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, itemClaimedBy8, subApproved, true},
		{"reviewer blocked from draft submission", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, itemClaimedBy7, subDraft, false},
		{"reviewer blocked from ai_reviewing", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, itemClaimedBy7, subAIReviewing, false},
		{"reviewer blocked when no submission", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, itemUnclaimed, nil, false},
		{"owner+reviewer (own task) sees via owner path", &auth.Claims{UserID: 10, Roles: []string{"owner", "reviewer"}}, itemUnclaimed, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanReadItem(tc.claims, task, tc.item, tc.submission); got != tc.want {
				t.Errorf("CanReadItem = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanReviewTask(t *testing.T) {
	task := model.Task{ID: 1, OwnerID: 10, Status: "published"}
	cases := []struct {
		name             string
		claims           *auth.Claims
		reviewerAssigned bool
		want             bool
	}{
		{"admin always", &auth.Claims{UserID: 1, Roles: []string{"admin"}}, false, true},
		{"owner blocked", &auth.Claims{UserID: 10, Roles: []string{"owner"}}, false, false},
		{"owner other task blocked", &auth.Claims{UserID: 11, Roles: []string{"owner"}}, false, false},
		{"reviewer assigned", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, true, true},
		{"reviewer unassigned blocked", &auth.Claims{UserID: 5, Roles: []string{"reviewer"}}, false, false},
		{"labeler blocked", &auth.Claims{UserID: 7, Roles: []string{"labeler"}}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanReviewTask(tc.claims, task, tc.reviewerAssigned); got != tc.want {
				t.Errorf("CanReviewTask = %v, want %v", got, tc.want)
			}
		})
	}
}
