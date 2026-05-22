package handler

import (
	"testing"
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
