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
)

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
