package submission

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"labelhub-api/internal/model"
)

const (
	uploadedFileStatusTemp     = "temp"
	uploadedFileStatusAttached = "attached"
)

var ErrInvalidUploadedFile = errors.New("submission: invalid uploaded file reference")

type fileUploadTemplateSchema struct {
	Fields []fileUploadTemplateField `json:"fields"`
}

// fileUploadTemplateField 既覆盖叶子字段(name + widget),也覆盖容器字段:
// Group 用 fields 嵌子字段,Tabs 用 tabs(每个 tab 各有 fields)。
// 渲染器递归渲染 Group / Tabs,附件归属扫描必须同样递归,否则嵌套层里的
// FileUpload 提交后不会绑定 revision,临时文件清理器会把真实证据当孤儿删掉(见 H-03)。
type fileUploadTemplateField struct {
	Name   string                    `json:"name"`
	Widget string                    `json:"widget"`
	Fields []fileUploadTemplateField `json:"fields"`
	Tabs   []fileUploadTemplateTab   `json:"tabs"`
}

type fileUploadTemplateTab struct {
	Fields []fileUploadTemplateField `json:"fields"`
}

const (
	widgetFileUpload = "FileUpload"
	widgetGroup      = "Group"
	widgetTabs       = "Tabs"
)

// collectFileUploadFieldNames 深度优先收集 schema 里所有 FileUpload 叶子字段的 name,
// 递归进入 Group.fields 与 Tabs.tabs[].fields。按遍历顺序去重返回,保证归属扫描稳定。
// 答案是扁平结构(渲染器把所有字段都按 name 写到答案根),因此用叶子 name 即可在答案里取值。
func collectFileUploadFieldNames(fields []fileUploadTemplateField) []string {
	names := make([]string, 0)
	seen := make(map[string]struct{})
	var walk func(fs []fileUploadTemplateField)
	walk = func(fs []fileUploadTemplateField) {
		for _, field := range fs {
			switch field.Widget {
			case widgetGroup:
				walk(field.Fields)
			case widgetTabs:
				for _, tab := range field.Tabs {
					walk(tab.Fields)
				}
			case widgetFileUpload:
				name := strings.TrimSpace(field.Name)
				if name == "" {
					continue
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				names = append(names, name)
			}
		}
	}
	walk(fields)
	return names
}

func attachUploadedFiles(tx *gorm.DB, task model.Task, sub model.Submission, revision model.SubmissionRevision, answerRaw []byte, userID uint64) error {
	keys, err := uploadedFileKeys(tx, task.ID, sub.TemplateVersion, answerRaw)
	if err != nil {
		return err
	}
	for _, key := range keys {
		var file model.UploadedFile
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("storage_key = ? AND task_id = ?", key, task.ID).
			First(&file).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidUploadedFile
			}
			return err
		}
		if file.TaskID != task.ID || file.CreatedBy != userID {
			return ErrInvalidUploadedFile
		}
		switch file.Status {
		case uploadedFileStatusTemp:
			res := tx.Model(&model.UploadedFile{}).
				Where("id = ? AND status = ?", file.ID, uploadedFileStatusTemp).
				Updates(map[string]any{
					"submission_revision_id": revision.ID,
					"status":                 uploadedFileStatusAttached,
					"attached_at":            NowUTC(),
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrInvalidUploadedFile
			}
		case uploadedFileStatusAttached:
			if !attachedToSubmissionHistory(tx, file, sub.ID) {
				return ErrInvalidUploadedFile
			}
		default:
			return ErrInvalidUploadedFile
		}
	}
	return nil
}

func attachedToSubmissionHistory(tx *gorm.DB, file model.UploadedFile, submissionID uint64) bool {
	if file.SubmissionRevisionID == nil {
		return false
	}
	var revision model.SubmissionRevision
	err := tx.Select("id").
		Where("id = ? AND submission_id = ?", *file.SubmissionRevisionID, submissionID).
		First(&revision).Error
	return err == nil
}

func uploadedFileKeys(tx *gorm.DB, taskID uint64, templateVersion int, answerRaw []byte) ([]string, error) {
	var answer map[string]any
	if err := json.Unmarshal(answerRaw, &answer); err != nil {
		return nil, err
	}
	if !hasPotentialUploadValue(answer) {
		return nil, nil
	}

	fieldNames, err := loadFileUploadFieldNames(tx, taskID, templateVersion)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	keys := make([]string, 0)
	for _, name := range fieldNames {
		fieldKeys, err := keysFromUploadAnswer(answer[name])
		if err != nil {
			return nil, err
		}
		for _, key := range fieldKeys {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func loadFileUploadFieldNames(tx *gorm.DB, taskID uint64, templateVersion int) ([]string, error) {
	var template model.TaskTemplate
	if err := tx.Where("task_id = ? AND version = ?", taskID, templateVersion).First(&template).Error; err != nil {
		return nil, err
	}
	var schema fileUploadTemplateSchema
	if err := json.Unmarshal([]byte(template.SchemaJSON), &schema); err != nil {
		return nil, err
	}
	return collectFileUploadFieldNames(schema.Fields), nil
}

func hasPotentialUploadValue(answer map[string]any) bool {
	for _, value := range answer {
		switch v := value.(type) {
		case string:
			if isLikelyStorageKey(strings.TrimSpace(v)) {
				return true
			}
		case []any:
			if len(v) > 0 {
				return true
			}
		}
	}
	return false
}

func isLikelyStorageKey(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func keysFromUploadAnswer(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case string:
		key := strings.TrimSpace(v)
		if key == "" {
			return nil, nil
		}
		return []string{key}, nil
	case []any:
		keys := make([]string, 0, len(v))
		for _, item := range v {
			raw, ok := item.(string)
			if !ok {
				return nil, ErrInvalidUploadedFile
			}
			key := strings.TrimSpace(raw)
			if key == "" {
				return nil, ErrInvalidUploadedFile
			}
			keys = append(keys, key)
		}
		return keys, nil
	default:
		return nil, ErrInvalidUploadedFile
	}
}
