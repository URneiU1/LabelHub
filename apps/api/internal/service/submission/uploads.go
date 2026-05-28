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

type fileUploadTemplateField struct {
	Name   string `json:"name"`
	Widget string `json:"widget"`
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

	var template model.TaskTemplate
	if err := tx.Where("task_id = ? AND version = ?", taskID, templateVersion).First(&template).Error; err != nil {
		return nil, err
	}

	var schema fileUploadTemplateSchema
	if err := json.Unmarshal([]byte(template.SchemaJSON), &schema); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	keys := make([]string, 0)
	for _, field := range schema.Fields {
		if field.Widget != "FileUpload" {
			continue
		}
		name := strings.TrimSpace(field.Name)
		if name == "" {
			continue
		}
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
