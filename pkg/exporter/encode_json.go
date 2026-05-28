package exporter

import "io"

// EncodeJSON 输出整体 JSON 数组,按 cols 顺序序列化每行(保序、关闭 HTML 转义)。
// 空行集合产出 "[]"。
func EncodeJSON(w io.Writer, cols []Column, rows []Row) (int, error) {
	buf := []byte{'['}
	for i, row := range rows {
		if i > 0 {
			buf = append(buf, ',')
		}
		var err error
		buf, err = appendRowObject(buf, cols, row)
		if err != nil {
			return i, err
		}
	}
	buf = append(buf, ']', '\n')
	if _, err := w.Write(buf); err != nil {
		return 0, err
	}
	return len(rows), nil
}
