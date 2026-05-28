package exporter

import (
	"io"

	"github.com/xuri/excelize/v2"
)

// EncodeXLSX 用 excelize StreamWriter 流式写出(扛大文件):首行表头(Export 名),
// 其后每行按 cols 顺序写单元格(嵌套值 stringify)。
func EncodeXLSX(w io.Writer, cols []Column, rows []Row) (int, error) {
	f := excelize.NewFile()
	defer f.Close()

	sw, err := f.NewStreamWriter("Sheet1")
	if err != nil {
		return 0, err
	}

	header := make([]any, len(cols))
	for i, col := range cols {
		header[i] = col.exportName()
	}
	if err := sw.SetRow("A1", header); err != nil {
		return 0, err
	}

	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			return i, err
		}
		vals := make([]any, len(cols))
		for j, col := range cols {
			vals[j] = stringifyCell(pick(row, col.Source))
		}
		if err := sw.SetRow(cell, vals); err != nil {
			return i, err
		}
	}

	if err := sw.Flush(); err != nil {
		return len(rows), err
	}
	if err := f.Write(w); err != nil {
		return len(rows), err
	}
	return len(rows), nil
}
