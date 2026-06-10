// Package envutil 提供环境变量读取的小工具。
package envutil

import "os"

// Default 返回环境变量 key 的值;为空(未设置或空串)时返回 fallback。
func Default(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
