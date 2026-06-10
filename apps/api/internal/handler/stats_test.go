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
	mock.ExpectQuery(`(?is)^SELECT .ai_score. FROM .submissions.`).WillReturnRows(
		sqlmock.NewRows([]string{"ai_score"}).AddRow(95.0).AddRow(72.0).AddRow(55.0))
	mock.ExpectQuery(`(?is)^SELECT DATE\(updated_at\).+FROM .submissions.`).WillReturnRows(
		sqlmock.NewRows([]string{"day", "count"}).AddRow("2026-05-31", 2).AddRow("2026-06-01", 2))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/1/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	// 混淆矩阵:pass→approve 出现 2 次、reject→approve 1 次
	confusion := data["confusion"].([]any)
	confCount := map[string]float64{}
	for _, cell := range confusion {
		m := cell.(map[string]any)
		confCount[m["ai"].(string)+"/"+m["human"].(string)] = m["count"].(float64)
	}
	if confCount["pass/approve"] != 2 || confCount["reject/approve"] != 1 {
		t.Fatalf("confusion = %v", confCount)
	}
	// 分数分桶:95→90-100、72→70-80、55→<60
	buckets := data["scoreBuckets"].([]any)
	bucketCount := map[string]float64{}
	for _, b := range buckets {
		m := b.(map[string]any)
		bucketCount[m["label"].(string)] = m["count"].(float64)
	}
	if bucketCount["90-100"] != 1 || bucketCount["70-80"] != 1 || bucketCount["<60"] != 1 {
		t.Fatalf("scoreBuckets = %v", bucketCount)
	}
	// 完成趋势:两天各 2 条
	trend := data["completionTrend"].([]any)
	if len(trend) != 2 {
		t.Fatalf("completionTrend len = %d, want 2", len(trend))
	}
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
	mock.ExpectQuery(`(?is)^SELECT .ai_score. FROM .submissions.`).WillReturnRows(
		sqlmock.NewRows([]string{"ai_score"}))
	mock.ExpectQuery(`(?is)^SELECT DATE\(updated_at\).+FROM .submissions.`).WillReturnRows(
		sqlmock.NewRows([]string{"day", "count"}))

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

func TestBuildConfusion(t *testing.T) {
	got := buildConfusion([]verdictPair{
		{"pass", "approve"},
		{"pass", "approve"},
		{"reject", "reject"},
		{"pass", "reject"},
	})
	idx := map[string]int{}
	for _, cell := range got {
		idx[cell.AI+"/"+cell.Human] = cell.Count
	}
	if idx["pass/approve"] != 2 || idx["reject/reject"] != 1 || idx["pass/reject"] != 1 {
		t.Fatalf("confusion = %v", idx)
	}
}

func TestBucketScores(t *testing.T) {
	got := bucketScores([]float64{100, 95, 90, 89.9, 60, 59.9, 0})
	idx := map[string]int{}
	for _, b := range got {
		idx[b.Label] = b.Count
	}
	// 100/95/90 → 90-100;89.9 → 80-90;60 → 60-70;59.9/0 → <60
	if idx["90-100"] != 3 || idx["80-90"] != 1 || idx["60-70"] != 1 || idx["<60"] != 2 {
		t.Fatalf("buckets = %v", idx)
	}
}
