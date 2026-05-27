package exporter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrTokenInvalid: token 结构/签名非法。ErrTokenExpired: 签名有效但已过期。
var (
	ErrTokenInvalid = errors.New("exporter: invalid download token")
	ErrTokenExpired = errors.New("exporter: download token expired")
)

// SignDownloadToken 生成下载 token: base64url(payload) + "." + base64url(HMAC-SHA256(secret, payload)),
// 其中 payload = "<exportID>.<expUnix>"。
func SignDownloadToken(secret string, exportID uint64, exp time.Time) string {
	payload := strconv.FormatUint(exportID, 10) + "." + strconv.FormatInt(exp.Unix(), 10)
	sig := signPayload(secret, payload)
	return encodeSegment([]byte(payload)) + "." + encodeSegment(sig)
}

// VerifyDownloadToken 常量时间校验签名;签名对但过期单独返回 ErrTokenExpired。
func VerifyDownloadToken(secret, token string, now time.Time) (uint64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return 0, ErrTokenInvalid
	}
	payload, err := decodeSegment(parts[0])
	if err != nil {
		return 0, ErrTokenInvalid
	}
	sig, err := decodeSegment(parts[1])
	if err != nil {
		return 0, ErrTokenInvalid
	}
	if !hmac.Equal(sig, signPayload(secret, string(payload))) {
		return 0, ErrTokenInvalid
	}
	fields := strings.Split(string(payload), ".")
	if len(fields) != 2 {
		return 0, ErrTokenInvalid
	}
	exportID, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, ErrTokenInvalid
	}
	expUnix, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, ErrTokenInvalid
	}
	if now.Unix() > expUnix {
		return exportID, ErrTokenExpired
	}
	return exportID, nil
}

func signPayload(secret, payload string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func encodeSegment(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func decodeSegment(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
