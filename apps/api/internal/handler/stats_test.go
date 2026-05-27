package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
)

func TestAggregateDimensions_AveragesByName(t *testing.T) {
	dims := []string{
		`[{"name":"相关性","score":8},{"name":"完整性","score":6}]`,
		`[{"name":"相关性","score":10}]`,
	}
	got := aggregateDimensions(dims)
	avg := map[string]float64{}
	for _, d := range got {
		avg[d.Name] = d.Avg
	}
	if avg["相关性"] != 9 || avg["完整性"] != 6 {
		t.Fatalf("unexpected averages: %v", avg)
	}
}

func TestAggregateDimensions_SkipsMalformed(t *testing.T) {
	got := aggregateDimensions([]string{`not json`, `[{"name":"x","score":4}]`, ``})
	if len(got) != 1 || got[0].Name != "x" || got[0].Avg != 4 {
		t.Fatalf("malformed should be skipped, got %v", got)
	}
}

func TestAiHumanAgree(t *testing.T) {
	cases := []struct {
		ai, human string
		want      bool
	}{
		{"pass", "approve", true},
		{"reject", "reject", true},
		{"pass", "reject", false},
		{"reject", "approve", false},
		{"uncertain", "approve", false},
	}
	for _, c := range cases {
		if aiHumanAgree(c.ai, c.human) != c.want {
			t.Fatalf("aiHumanAgree(%s,%s) != %v", c.ai, c.human, c.want)
		}
	}
}

func TestTaskStats_AggregatesCounts(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "owner_id", "status", "total_items", "finished_items"}).AddRow(1, 7, "published", 10, 4))
	mock.ExpectQuery(`(?is)^SELECT status, COUNT.+FROM .submissions. WHERE task_id`).WillReturnRows(
		sqlmock.NewRows([]string{"status", "count"}).AddRow("approved", 3).AddRow("rejected", 1).AddRow("human_reviewing", 2))
	mock.ExpectQuery(`(?is)^SELECT ai_verdict, human_verdict FROM .submissions.`).WillReturnRows(
		sqlmock.NewRows([]string{"ai_verdict", "human_verdict"}).AddRow("pass", "approve").AddRow("reject", "approve").AddRow("pass", "approve"))
	mock.ExpectQuery(`(?is)^SELECT ai_reviews.dimensions FROM .ai_reviews. JOIN.+ai_reviews.revision_id = submissions.current_revision_id`).WillReturnRows(
		sqlmock.NewRows([]string{"dimensions"}).
			AddRow(`[{"name":"相关性","score":8},{"name":"完整性","score":6}]`).
			AddRow(`[{"name":"相关性","score":10}]`))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	progress := data["progress"].(map[string]any)
	if progress["total"] != float64(10) || progress["finished"] != float64(4) {
		t.Fatalf("progress = %v", progress)
	}
	if data["passRate"] != 0.75 {
		t.Fatalf("passRate = %v, want 0.75", data["passRate"])
	}
	ai := data["aiVsHuman"].(map[string]any)
	if ai["compared"] != float64(3) || ai["disagree"] != float64(1) {
		t.Fatalf("aiVsHuman = %v", ai)
	}
	dims := data["dimensionAverages"].([]any)
	avg := map[string]float64{}
	for _, d := range dims {
		m := d.(map[string]any)
		avg[m["name"].(string)] = m["avg"].(float64)
	}
	if avg["相关性"] != 9 || avg["完整性"] != 6 {
		t.Fatalf("dimensionAverages = %v", avg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTaskStats_ZeroDivisionSafe(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "owner_id", "status", "total_items", "finished_items"}).AddRow(1, 7, "draft", 0, 0))
	mock.ExpectQuery(`(?is)^SELECT status, COUNT.+FROM .submissions. WHERE task_id`).WillReturnRows(
		sqlmock.NewRows([]string{"status", "count"}))
	mock.ExpectQuery(`(?is)^SELECT ai_verdict, human_verdict FROM .submissions.`).WillReturnRows(
		sqlmock.NewRows([]string{"ai_verdict", "human_verdict"}))
	mock.ExpectQuery(`(?is)^SELECT ai_reviews.dimensions FROM .ai_reviews. JOIN.+ai_reviews.revision_id = submissions.current_revision_id`).WillReturnRows(
		sqlmock.NewRows([]string{"dimensions"}))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["passRate"] != float64(0) {
		t.Fatalf("passRate should be 0, got %v", data["passRate"])
	}
}
