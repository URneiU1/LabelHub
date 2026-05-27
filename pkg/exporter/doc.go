// Package exporter 是跨模块共享的导出核心(api 与 ai-worker 都 import 它),
// 用裸 *sql.DB 不依赖 gorm / labelhub-api internal 包,范式对齐 pkg/llmreview。
//
// 职责:多格式编码器(JSON/JSONL/CSV/XLSX[/MD])、字段映射、行加载、
// 异步执行核心 Run()、HMAC 下载 token。
package exporter
