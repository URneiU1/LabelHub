package exporter

import (
	"errors"
	"io"
)

// ErrUnsupportedFormat: 未知导出格式。
var ErrUnsupportedFormat = errors.New("exporter: unsupported format")

// Encode 按 format 分发到具体编码器,返回写出的行数。
func Encode(format string, w io.Writer, cols []Column, rows []Row) (int, error) {
	switch format {
	case "json":
		return EncodeJSON(w, cols, rows)
	case "jsonl":
		return EncodeJSONL(w, cols, rows)
	case "csv":
		return EncodeCSV(w, cols, rows)
	case "xlsx":
		return EncodeXLSX(w, cols, rows)
	case "md":
		return EncodeMarkdown(w, cols, rows)
	case "coco":
		return EncodeCOCO(w, cols, rows)
	case "sft":
		return EncodeSFT(w, cols, rows)
	case "dpo":
		return EncodeDPO(w, cols, rows)
	default:
		return 0, ErrUnsupportedFormat
	}
}

// SupportedFormat 判断 format 是否受支持(handler 校验白名单用)。
func SupportedFormat(format string) bool {
	switch format {
	case "json", "jsonl", "csv", "xlsx", "md", "coco", "sft", "dpo":
		return true
	default:
		return false
	}
}

// FileExtension 返回带点的文件后缀。
func FileExtension(format string) string {
	switch format {
	case "json":
		return ".json"
	case "jsonl":
		return ".jsonl"
	case "csv":
		return ".csv"
	case "xlsx":
		return ".xlsx"
	case "md":
		return ".md"
	case "coco":
		return ".coco.json"
	case "sft":
		return ".sft.jsonl"
	case "dpo":
		return ".dpo.jsonl"
	default:
		return ".bin"
	}
}

// ContentType 返回下载时的 Content-Type。
func ContentType(format string) string {
	switch format {
	case "json":
		return "application/json; charset=utf-8"
	case "jsonl":
		return "application/x-ndjson; charset=utf-8"
	case "csv":
		return "text/csv; charset=utf-8"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "md":
		return "text/markdown; charset=utf-8"
	case "coco":
		return "application/json; charset=utf-8"
	case "sft", "dpo":
		return "application/x-ndjson; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
