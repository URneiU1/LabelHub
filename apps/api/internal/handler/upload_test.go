package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/model"
)

// M-05:被授权的 reviewer 应能下载 needs_arbitration submission 的证据附件。
// 直接测 canDownloadUpload 的授权判定,避开磁盘文件不存在导致的 FileAttachment 404。
func TestCanDownloadUploadAllowsAssignedReviewerForArbitration(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	// reviewer 被指派到该 task(task_reviewers 命中)。
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// 附件挂在该 task 下一个 needs_arbitration submission 的 revision 上 → 命中状态白名单。
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .submission_revisions. JOIN submissions.+status IN`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	h := UploadHandler{db: db}
	revID := uint64(901)
	allowed, err := h.canDownloadUpload(
		&auth.Claims{UserID: 7, Username: "reviewer1", Roles: []string{"reviewer"}},
		model.Task{ID: 1, OwnerID: 99},
		model.UploadedFile{ID: 301, TaskID: 1, CreatedBy: 8, SubmissionRevisionID: &revID},
	)
	if err != nil {
		t.Fatalf("canDownloadUpload error: %v", err)
	}
	if !allowed {
		t.Fatal("assigned reviewer should be allowed to download needs_arbitration evidence")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// storageKey 必须能去重:相邻调用产出不同 key,因为含纳秒时间戳。
func TestStorageKeyUnique(t *testing.T) {
	a := storageKey("photo.png")
	b := storageKey("photo.png")
	if a == b {
		t.Fatalf("storageKey collision on consecutive calls: %q == %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("storageKey should be 64-char hex (sha256), got len=%d", len(a))
	}
}

// MIME 白名单边界:PLAN §3.1 规定 png/jpeg/webp/pdf/txt/json。
func TestUploadMIMEWhitelist(t *testing.T) {
	good := []string{"image/png", "image/jpeg", "image/webp", "application/pdf", "text/plain", "application/json"}
	bad := []string{"text/html", "application/octet-stream", "image/gif", ""}

	for _, mime := range good {
		if _, ok := allowedUploadMIME[mime]; !ok {
			t.Errorf("MIME %q should be allowed but is not", mime)
		}
	}
	for _, mime := range bad {
		if _, ok := allowedUploadMIME[mime]; ok {
			t.Errorf("MIME %q should NOT be allowed but is", mime)
		}
	}

	if _, ok := imageMIMEs["image/png"]; !ok {
		t.Error("image/png must be classified as image to enforce 5MB limit")
	}
	if _, ok := imageMIMEs["application/pdf"]; ok {
		t.Error("application/pdf must NOT be image (5MB image cap should not apply to PDF)")
	}
}

func TestUploadSampleMatchesDeclaredMIME(t *testing.T) {
	if !uploadSampleMatches("application/pdf", []byte("%PDF-1.7\n")) {
		t.Fatal("pdf sample should match application/pdf")
	}
	if !uploadSampleMatches("application/json", []byte(`{"ok":true}`)) {
		t.Fatal("json sample should match application/json")
	}
	if uploadSampleMatches("application/json", []byte(`{"ok":`)) {
		t.Fatal("invalid json must not match application/json")
	}
	if uploadSampleMatches("image/png", []byte("<script>alert(1)</script>")) {
		t.Fatal("html/script sample must not match image/png")
	}
	if uploadSampleMatches("application/json", []byte(`<html></html>`)) {
		t.Fatal("html sample must not match application/json")
	}
}

func TestUploadOwnerCannotUploadToOtherTask(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 99, "published"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "owner1", Roles: []string{"owner"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, multipartUploadRequest("/uploads", "1", "note.txt", "text/plain", []byte("hello")))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

// IDOR 回归守卫:未被指派到该 task 的 reviewer 不能上传文件,
// 即使该 task 存在 human_reviewing 提交。canUploadToTask 的 reviewer 分支
// 必须先过 canReviewTask(task_reviewers 绑定校验)。
func TestUploadReviewerNotAssignedForbidden(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(2, 99, "published"))
	// canReviewTask 查 task_reviewers:未绑定 -> count 0 -> CanReviewTask 返回 false -> 403,
	// 且不会再查 submissions(被 !assigned 提前 return)。
	mock.ExpectQuery(`(?is)^SELECT count.+FROM .task_reviewers.`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, multipartUploadRequest("/uploads", "2", "note.txt", "text/plain", []byte("hello")))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for reviewer not assigned to task, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestDownloadUploadRequiresTaskAccess(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "original_name", "status", "created_by"}).
			AddRow(301, 1, "file.txt", "file.txt", "attached", 8))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 99, "published"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "labeler1", Roles: []string{"labeler"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/uploads/301", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func TestDownloadUploadRejectsReviewerWithoutAttachedRevision(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`(?is)^SELECT.+FROM .uploaded_files.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "storage_key", "original_name", "status", "created_by"}).
			AddRow(301, 1, "file.txt", "file.txt", "temp", 8))
	mock.ExpectQuery(`(?is)^SELECT.+FROM .tasks.`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_id", "status"}).
			AddRow(1, 99, "published"))

	r := newGinWithClaims(&auth.Claims{UserID: 7, Username: "reviewer1", Roles: []string{"reviewer"}})
	registerAllHandlers(r, db)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/uploads/301", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations not met: %v", err)
	}
}

func multipartUploadRequest(path string, taskID string, filename string, contentType string, content []byte) *http.Request {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("task_id", taskID)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, _ := writer.CreatePart(header)
	_, _ = part.Write(content)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
