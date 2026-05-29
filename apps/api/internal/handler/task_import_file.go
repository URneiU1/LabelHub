package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"labelhub-api/internal/httpx"
)

// 支持的导入文件格式。
const (
	importFormatJSON  = "json"
	importFormatJSONL = "jsonl"
	importFormatXLSX  = "xlsx"
)

var errEmptyImportFile = errors.New("import file is empty")

// ImportItemsFile 从上传文件导入题目,支持 JSON 数组 / {items:[...]} / JSONL / Excel(.xlsx)。
// multipart: file(必填) + format(可选,缺省按文件扩展名推断)。复用 insertImportedItems 的去重逻辑。
func (h TaskHandler) ImportItemsFile(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportFileBytes+4096)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isRequestTooLarge(err) {
			httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "import file is too large")
			return
		}
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file is required")
		return
	}
	if fileHeader.Size <= 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "file must not be empty")
		return
	}
	if fileHeader.Size > maxImportFileBytes {
		httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "import file is too large")
		return
	}

	format := strings.ToLower(strings.TrimSpace(c.PostForm("format")))
	if format == "" {
		format = formatFromFilename(fileHeader.Filename)
	}

	src, err := fileHeader.Open()
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read upload")
		return
	}
	defer src.Close()
	content, err := io.ReadAll(io.LimitReader(src, maxImportFileBytes+1))
	if err != nil || int64(len(content)) > maxImportFileBytes {
		httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "import file is too large")
		return
	}

	var items []map[string]any
	var parseErr error
	switch format {
	case importFormatJSON:
		items, parseErr = parseJSONImport(content)
	case importFormatJSONL:
		items, parseErr = parseJSONLImport(content)
	case importFormatXLSX:
		items, parseErr = parseXLSXImport(content)
	default:
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "format must be one of json, jsonl, xlsx")
		return
	}
	if parseErr != nil {
		if errors.Is(parseErr, errEmptyImportFile) {
			httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "import file contains no items")
			return
		}
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "failed to parse "+format+" import file")
		return
	}
	if len(items) == 0 {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "import file contains no items")
		return
	}
	if len(items) > maxImportItems {
		httpx.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "too many items in a single import")
		return
	}

	if err := insertImportedItems(h.db, task.ID, items); err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to import items")
		return
	}
	httpx.OK(c, gin.H{"imported": len(items), "format": format})
}

func formatFromFilename(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jsonl", ".ndjson":
		return importFormatJSONL
	case ".xlsx":
		return importFormatXLSX
	case ".json":
		return importFormatJSON
	default:
		return importFormatJSON
	}
}

// parseJSONImport 接受一个 JSON 数组,或 {"items":[...]} 包装。
func parseJSONImport(content []byte) ([]map[string]any, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil, errEmptyImportFile
	}
	if trimmed[0] == '[' {
		var arr []map[string]any
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	var wrapper struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(trimmed, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Items, nil
}

// parseJSONLImport 每行一个 JSON 对象(空行跳过)。
func parseJSONLImport(content []byte) ([]map[string]any, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	// 单行可能很长(大 payload),放宽 token 上限到 1MB。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	items := make([]map[string]any, 0)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			return nil, err
		}
		items = append(items, obj)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// parseXLSXImport 读首个工作表,首行作为字段名,其余每行映射成一个 payload 对象。
func parseXLSXImport(content []byte) ([]map[string]any, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errEmptyImportFile
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errEmptyImportFile
	}

	headers := rows[0]
	items := make([]map[string]any, 0, len(rows)-1)
	for _, row := range rows[1:] {
		obj := make(map[string]any, len(headers))
		hasValue := false
		for col, header := range headers {
			header = strings.TrimSpace(header)
			if header == "" {
				continue
			}
			var cell string
			if col < len(row) {
				cell = row[col]
			}
			if cell != "" {
				hasValue = true
			}
			obj[header] = cell
		}
		// 跳过整行空白,避免 Excel 末尾空行被导入。
		if hasValue {
			items = append(items, obj)
		}
	}
	return items, nil
}
