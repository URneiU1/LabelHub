package exporter

import (
	"io"
	"strings"
)

// EncodeMarkdown 输出标准 GFM 表格:表头行(Export 名)+ "---" 分隔行 + 每行数据。
// 单元格内的 | 与换行会被转义(\| / <br>),嵌套值(map/array)JSON 序列化成字符串。
// 返回写出的数据行数(不含表头与分隔行)。
//
// Outputs a standard GFM table: header row + separator row + one row per record.
func EncodeMarkdown(w io.Writer, cols []Column, rows []Row) (int, error) {
	var b strings.Builder

	b.WriteByte('|')
	for _, col := range cols {
		b.WriteByte(' ')
		b.WriteString(mdSafeCell(col.exportName()))
		b.WriteString(" |")
	}
	b.WriteByte('\n')

	b.WriteByte('|')
	for range cols {
		b.WriteString(" --- |")
	}
	b.WriteByte('\n')

	for _, row := range rows {
		b.WriteByte('|')
		for _, col := range cols {
			b.WriteByte(' ')
			b.WriteString(mdSafeCell(stringifyCell(pick(row, col.Source))))
			b.WriteString(" |")
		}
		b.WriteByte('\n')
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// mdSafeCell 转义会破坏 GFM 表格结构的字符:
// | 转成 \|(否则被当列分隔符),换行(\r\n / \r / \n)转成 <br>(否则截断当前行)。
func mdSafeCell(s string) string {
	if s == "" {
		return s
	}
	r := strings.NewReplacer(
		"|", `\|`,
		"\r\n", "<br>",
		"\r", "<br>",
		"\n", "<br>",
	)
	return r.Replace(s)
}
