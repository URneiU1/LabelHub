package exporter

import (
	"encoding/json"
	"io"
	"strings"
)

// cocoDoc 是 COCO 风格导出的顶层文档。LabelHub 是通用表单标注(无检测框坐标),
// 因此 categories 固定为单一通用类目,标注内容放进 annotation.attributes。
type cocoDoc struct {
	Info        cocoInfo          `json:"info"`
	Images      []json.RawMessage `json:"images"`
	Annotations []json.RawMessage `json:"annotations"`
	Categories  []cocoCategory    `json:"categories"`
}

type cocoInfo struct {
	Description string `json:"description"`
	Source      string `json:"source"`
}

type cocoCategory struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Supercategory string `json:"supercategory"`
}

// cocoImageObj / cocoAnnotationObj 用 RawMessage 承载已按列序编码的片段,保证输出键序稳定。
type cocoImageObj struct {
	ID       json.RawMessage `json:"id"`
	FileName string          `json:"file_name"`
	Payload  json.RawMessage `json:"labelhub_payload"`
}

type cocoAnnotationObj struct {
	ID         json.RawMessage `json:"id"`
	ImageID    json.RawMessage `json:"image_id"`
	CategoryID int             `json:"category_id"`
	Attributes json.RawMessage `json:"attributes"`
}

// EncodeCOCO 输出 COCO 风格的单文档 JSON:每个题目(item)映射成 image,每条 approved
// 提交映射成 annotation,标注答案按 cols 字段映射(选列/重命名)放进 annotation.attributes。
// image 按 item_id 去重(overlap 任务同一题可能有多条 approved 提交)。
func EncodeCOCO(w io.Writer, cols []Column, rows []Row) (int, error) {
	doc := cocoDoc{
		Info:        cocoInfo{Description: "LabelHub annotation export (COCO-style)", Source: "labelhub"},
		Images:      make([]json.RawMessage, 0, len(rows)),
		Annotations: make([]json.RawMessage, 0, len(rows)),
		Categories:  []cocoCategory{{ID: 1, Name: "annotation", Supercategory: "labelhub"}},
	}

	seenItems := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		itemID, err := marshalValue(pick(row, "item_id"))
		if err != nil {
			return 0, err
		}
		if len(itemID) == 0 || string(itemID) == "null" {
			// No usable item_id → skip the row entirely: never emit a ghost image/annotation
			// keyed on a null id. Unreachable on the product export path (the rows JOIN
			// guarantees item_id); this is library-level robustness for any caller.
			continue
		}
		if _, seen := seenItems[string(itemID)]; !seen {
			seenItems[string(itemID)] = struct{}{}
			payload, err := marshalValue(pick(row, "payload"))
			if err != nil {
				return 0, err
			}
			// 用 marshalValue 而非 json.Marshal:后者会对 RawMessage 重新 HTML 转义。
			img, err := marshalValue(cocoImageObj{
				ID:       itemID,
				FileName: cocoFileName(row, itemID),
				Payload:  payload,
			})
			if err != nil {
				return 0, err
			}
			doc.Images = append(doc.Images, img)
		}

		subID, err := marshalValue(pick(row, "submission_id"))
		if err != nil {
			return 0, err
		}
		attrs, err := appendRowObject(nil, cols, row)
		if err != nil {
			return 0, err
		}
		ann, err := marshalValue(cocoAnnotationObj{
			ID:         subID,
			ImageID:    itemID,
			CategoryID: 1,
			Attributes: attrs,
		})
		if err != nil {
			return 0, err
		}
		doc.Annotations = append(doc.Annotations, ann)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// cocoFileName 优先用 external_id 作为 file_name;缺失时退化为 "item-<item_id>"。
func cocoFileName(row Row, itemID json.RawMessage) string {
	if ext, ok := pick(row, "external_id").(string); ok && ext != "" {
		return ext
	}
	return "item-" + strings.Trim(string(itemID), `"`)
}
