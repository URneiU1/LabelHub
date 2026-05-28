package exporter

import (
	"encoding/csv"
	"io"
)

// utf8BOM 让 Excel 正确按 UTF-8 识别中文。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// EncodeCSV 先写 UTF-8 BOM,再用 encoding/csv 写表头(Export 名)+ 数据行;
// 嵌套值(map/array)被 JSON 序列化成字符串塞进单元格。
func EncodeCSV(w io.Writer, cols []Column, rows []Row) (int, error) {
	if _, err := w.Write(utf8BOM); err != nil {
		return 0, err
	}
	cw := csv.NewWriter(w)

	header := make([]string, len(cols))
	for i, col := range cols {
		header[i] = col.exportName()
	}
	if err := cw.Write(header); err != nil {
		return 0, err
	}

	for i, row := range rows {
		record := make([]string, len(cols))
		for j, col := range cols {
			record[j] = csvSafeCell(stringifyCell(pick(row, col.Source)))
		}
		if err := cw.Write(record); err != nil {
			return i, err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return len(rows), err
	}
	return len(rows), nil
}

// csvSafeCell 防 CSV 公式注入:以 = + - @ \t \r 开头的单元格会被 Excel / LibreOffice
// 当公式执行(如用户答案 =HYPERLINK(...));给这类值加前导单引号,使其被当作纯文本。
func csvSafeCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
