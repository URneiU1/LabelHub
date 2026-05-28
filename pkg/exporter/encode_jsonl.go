package exporter

import "io"

// EncodeJSONL 输出一行一个 JSON 对象,每行(含末行)以 \n 结尾。
func EncodeJSONL(w io.Writer, cols []Column, rows []Row) (int, error) {
	for i, row := range rows {
		line, err := appendRowObject(nil, cols, row)
		if err != nil {
			return i, err
		}
		line = append(line, '\n')
		if _, err := w.Write(line); err != nil {
			return i, err
		}
	}
	return len(rows), nil
}
