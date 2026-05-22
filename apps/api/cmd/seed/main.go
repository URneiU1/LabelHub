package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	"labelhub-api/internal/auth"
	"labelhub-api/internal/db"
	"labelhub-api/internal/model"
)

type seedUser struct {
	username    string
	displayName string
	email       string
	roles       []string
}

func main() {
	database := db.Init()
	db.RunMigrations()

	users := []seedUser{
		{username: "owner1", displayName: "任务负责人一号", email: "owner1@example.com", roles: []string{"owner", "reviewer"}},
		{username: "owner2", displayName: "任务负责人二号", email: "owner2@example.com", roles: []string{"owner"}},
		{username: "labeler1", displayName: "标注员一号", email: "labeler1@example.com", roles: []string{"labeler"}},
		{username: "labeler2", displayName: "标注员二号", email: "labeler2@example.com", roles: []string{"labeler"}},
		{username: "labeler3", displayName: "标注员三号", email: "labeler3@example.com", roles: []string{"labeler"}},
		{username: "reviewer1", displayName: "审核员一号", email: "reviewer1@example.com", roles: []string{"reviewer"}},
		{username: "reviewer2", displayName: "审核员二号", email: "reviewer2@example.com", roles: []string{"reviewer"}},
		{username: "admin1", displayName: "管理员", email: "admin1@example.com", roles: []string{"admin"}},
		{username: "system_ai", displayName: "AI 审核 Agent", email: "system@example.com", roles: []string{"system"}},
	}

	for _, user := range users {
		if err := upsertUser(database, user); err != nil {
			log.Fatalf("seed user %s: %v", user.username, err)
		}
	}
	if err := seedQAQuality(database); err != nil {
		log.Fatalf("seed qa_quality: %v", err)
	}

	log.Println("seed ready; demo password is pass")
}

func upsertUser(database *gorm.DB, seed seedUser) error {
	passwordHash, err := auth.HashPassword("pass")
	if err != nil {
		return err
	}

	return database.Transaction(func(tx *gorm.DB) error {
		user := model.User{Username: seed.username}
		if err := tx.Where(model.User{Username: seed.username}).
			Attrs(model.User{
				PasswordHash: passwordHash,
				DisplayName:  seed.displayName,
				Email:        seed.email,
				Status:       "active",
			}).
			FirstOrCreate(&user).Error; err != nil {
			return err
		}

		if err := tx.Model(&user).Updates(map[string]any{
			"display_name": seed.displayName,
			"email":        seed.email,
			"status":       "active",
		}).Error; err != nil {
			return err
		}

		for _, role := range seed.roles {
			var count int64
			if err := tx.Model(&model.UserRole{}).
				Where("user_id = ? AND role = ?", user.ID, role).
				Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := tx.Create(&model.UserRole{UserID: user.ID, Role: role}).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func seedQAQuality(database *gorm.DB) error {
	return database.Transaction(func(tx *gorm.DB) error {
		var owner model.User
		if err := tx.Where("username = ?", "owner1").First(&owner).Error; err != nil {
			return err
		}
		var reviewer model.User
		if err := tx.Where("username = ?", "reviewer1").First(&reviewer).Error; err != nil {
			return err
		}

		baseline, err := os.ReadFile(projectFile("tools/seed/datasets/qa_quality/标注要求.md"))
		if err != nil {
			return err
		}
		task := model.Task{
			OwnerID:             owner.ID,
			Title:               "官方 qa_quality 质检标注",
			Description:         model.StringFrom("基于官方 qa_quality 数据集的问答质量标注任务。"),
			BaselineDescription: model.StringFrom(string(baseline)),
			Status:              "published",
			Distribution:        "first_come",
			AIReviewEnabled:     false,
			HumanReviewEnabled:  true,
			PublishedAt:         model.TimeFrom(time.Now().UTC()),
		}
		if err := tx.Where("title = ?", task.Title).Attrs(task).FirstOrCreate(&task).Error; err != nil {
			return err
		}
		if err := tx.Model(&task).Updates(map[string]any{
			"baseline_description": string(baseline),
			"status":               "published",
			"ai_review_enabled":    false,
			"human_review_enabled": true,
			"published_at":         time.Now().UTC(),
		}).Error; err != nil {
			return err
		}

		schema, err := os.ReadFile(projectFile("tools/seed/templates/qa_quality_review.json"))
		if err != nil {
			return err
		}
		hash := sha256.Sum256(schema)
		template := model.TaskTemplate{
			TaskID:     task.ID,
			Version:    1,
			SchemaJSON: string(schema),
			SchemaHash: hex.EncodeToString(hash[:]),
			CreatedBy:  owner.ID,
		}
		var existing model.TaskTemplate
		err = tx.Where("task_id = ? AND version = ?", task.ID, 1).First(&existing).Error
		var existingPtr *model.TaskTemplate
		if err == nil {
			existingPtr = &existing
			template.ID = existing.ID
		}
		action := seedTemplateActions(task, existingPtr, template.SchemaHash)
		switch {
		case err == nil:
			if action.UpdateExisting {
				if err := tx.Model(&existing).Updates(map[string]any{
					"schema_json": template.SchemaJSON,
					"schema_hash": template.SchemaHash,
				}).Error; err != nil {
					return err
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if action.CreateTemplate {
				if err := tx.Create(&template).Error; err != nil {
					return err
				}
			}
		default:
			return err
		}
		if !action.CreateTemplate && template.ID == 0 {
			return errors.New("seed template action resolved without template id")
		}
		if err := tx.Where("task_id = ? AND user_id = ?", task.ID, reviewer.ID).
			Attrs(model.TaskReviewer{TaskID: task.ID, UserID: reviewer.ID, AssignedBy: &owner.ID, AssignedAt: time.Now().UTC()}).
			FirstOrCreate(&model.TaskReviewer{}).Error; err != nil {
			return err
		}
		// Seed 只负责补齐/更新官方 v1 模板。S2 之后 Owner 可能已经创建 v2/v3,
		// rerun seed 不能把当前模板指针回滚到 v1。
		if action.AttachToTask {
			if err := tx.Model(&task).Update("template_id", template.ID).Error; err != nil {
				return err
			}
		}

		items, err := loadQAQualityItems()
		if err != nil {
			return err
		}
		for _, payload := range items {
			externalID, _ := payload["id"].(string)
			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			item := model.TaskItem{
				TaskID:     task.ID,
				ExternalID: model.StringFrom(externalID),
				Payload:    string(raw),
				Status:     "available",
			}
			if err := tx.Where("task_id = ? AND external_id = ?", task.ID, externalID).Attrs(item).FirstOrCreate(&item).Error; err != nil {
				return err
			}
			if err := tx.Model(&item).Updates(map[string]any{"payload": string(raw)}).Error; err != nil {
				return err
			}
		}

		return tx.Model(&task).Update("total_items", len(items)).Error
	})
}

type seedTemplateAction struct {
	CreateTemplate bool
	UpdateExisting bool
	AttachToTask   bool
}

func seedTemplateActions(task model.Task, existing *model.TaskTemplate, nextHash string) seedTemplateAction {
	return seedTemplateAction{
		CreateTemplate: existing == nil,
		UpdateExisting: existing != nil && existing.SchemaHash != nextHash,
		AttachToTask:   task.TemplateID == nil,
	}
}

func loadQAQualityItems() ([]map[string]any, error) {
	raw, err := os.ReadFile(projectFile("tools/seed/datasets/qa_quality/json/qa_quality.json"))
	if err != nil {
		return nil, err
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func projectFile(path string) string {
	return filepath.Join("../..", path)
}
