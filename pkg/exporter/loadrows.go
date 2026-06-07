package exporter

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// decodeJSONFallback 把 JSON 字符串解码成 any;数字用 UseNumber 保大整数精度(S3 教训);
// 解码失败原样返回字符串(容忍历史脏数据,不整批崩)。
func decodeJSONFallback(raw string) any {
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw
	}
	return v
}

// LoadApprovedRows 用裸 SQL 取某任务 approved submissions 的扁平行(api 与 worker 共用)。
// includeReviews 时附加最新 ai_review.* / human_review.* 扁平点号键(缺审核填 nil,保列稳定)。
func LoadApprovedRows(ctx context.Context, db *sql.DB, taskID uint64, includeReviews bool) ([]Row, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, ti.id, ti.external_id, ti.payload, sr.answer
		FROM submissions s
		JOIN task_items ti ON ti.id = s.item_id
		JOIN submission_revisions sr ON sr.id = s.current_revision_id
		WHERE s.task_id = ? AND s.status = 'approved'
		ORDER BY s.id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type base struct {
		subID, itemID   uint64
		externalID      sql.NullString
		payload, answer string
	}
	var bases []base
	var subIDs []uint64
	for rows.Next() {
		var b base
		if err := rows.Scan(&b.subID, &b.itemID, &b.externalID, &b.payload, &b.answer); err != nil {
			return nil, err
		}
		bases = append(bases, b)
		subIDs = append(subIDs, b.subID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var aiBy map[uint64]aiReviewRow
	var humanBy map[uint64]humanReviewRow
	if includeReviews && len(subIDs) > 0 {
		if aiBy, err = latestAIReviews(ctx, db, subIDs); err != nil {
			return nil, err
		}
		if humanBy, err = latestHumanReviews(ctx, db, subIDs); err != nil {
			return nil, err
		}
	}

	out := make([]Row, 0, len(bases))
	for _, b := range bases {
		row := Row{
			{Key: "submission_id", Value: b.subID},
			{Key: "item_id", Value: b.itemID},
			{Key: "external_id", Value: nullStrToAny(b.externalID)},
			{Key: "payload", Value: decodeJSONFallback(b.payload)},
			{Key: "answer", Value: decodeJSONFallback(b.answer)},
		}
		if includeReviews {
			ai := aiBy[b.subID]
			h := humanBy[b.subID]
			row = append(row,
				Cell{"ai_review.verdict", nullStrToAny(ai.verdict)},
				Cell{"ai_review.overall_score", nullFloatToAny(ai.overallScore)},
				Cell{"ai_review.dimensions", jsonOrNil(ai.dimensions)},
				Cell{"ai_review.reason", nullStrToAny(ai.reason)},
				Cell{"ai_review.prompt_config_id", nullIntToAny(ai.promptConfigID)},
				Cell{"ai_review.prompt_version", nullIntToAny(ai.promptVersion)},
				Cell{"ai_review.created_at", nullTimeToAny(ai.createdAt)},
				Cell{"human_review.verdict", nullStrToAny(h.verdict)},
				Cell{"human_review.reason", nullStrToAny(h.reason)},
				Cell{"human_review.stage", nullStrToAny(h.stage)},
				Cell{"human_review.reviewer_id", nullIntToAny(h.reviewerID)},
				Cell{"human_review.created_at", nullTimeToAny(h.createdAt)},
			)
		}
		out = append(out, row)
	}
	return out, nil
}

type aiReviewRow struct {
	verdict        sql.NullString
	overallScore   sql.NullFloat64
	dimensions     sql.NullString
	reason         sql.NullString
	promptConfigID sql.NullInt64
	promptVersion  sql.NullInt64
	createdAt      sql.NullTime
}

type humanReviewRow struct {
	verdict    sql.NullString
	reason     sql.NullString
	stage      sql.NullString
	reviewerID sql.NullInt64
	createdAt  sql.NullTime
}

func latestAIReviews(ctx context.Context, db *sql.DB, subIDs []uint64) (map[uint64]aiReviewRow, error) {
	q := `SELECT submission_id, verdict, overall_score, dimensions, reason, prompt_config_id, prompt_version, created_at
	      FROM ai_reviews WHERE submission_id IN (` + inPlaceholders(len(subIDs)) + `) ORDER BY id DESC`
	rows, err := db.QueryContext(ctx, q, toArgs(subIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uint64]aiReviewRow, len(subIDs))
	for rows.Next() {
		var sid uint64
		var r aiReviewRow
		if err := rows.Scan(&sid, &r.verdict, &r.overallScore, &r.dimensions, &r.reason, &r.promptConfigID, &r.promptVersion, &r.createdAt); err != nil {
			return nil, err
		}
		if _, seen := out[sid]; !seen {
			out[sid] = r
		}
	}
	return out, rows.Err()
}

func latestHumanReviews(ctx context.Context, db *sql.DB, subIDs []uint64) (map[uint64]humanReviewRow, error) {
	q := `SELECT submission_id, verdict, reason, stage, reviewer_id, created_at
	      FROM human_reviews WHERE submission_id IN (` + inPlaceholders(len(subIDs)) + `) ORDER BY id DESC`
	rows, err := db.QueryContext(ctx, q, toArgs(subIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uint64]humanReviewRow, len(subIDs))
	for rows.Next() {
		var sid uint64
		var r humanReviewRow
		if err := rows.Scan(&sid, &r.verdict, &r.reason, &r.stage, &r.reviewerID, &r.createdAt); err != nil {
			return nil, err
		}
		if _, seen := out[sid]; !seen {
			out[sid] = r
		}
	}
	return out, rows.Err()
}

func inPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimPrefix(strings.Repeat(",?", n), ",")
}

func toArgs(ids []uint64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

func nullStrToAny(n sql.NullString) any {
	if n.Valid {
		return n.String
	}
	return nil
}

func nullFloatToAny(n sql.NullFloat64) any {
	if n.Valid {
		return n.Float64
	}
	return nil
}

func nullIntToAny(n sql.NullInt64) any {
	if n.Valid {
		return n.Int64
	}
	return nil
}

func nullTimeToAny(n sql.NullTime) any {
	if n.Valid {
		return n.Time
	}
	return nil
}

func jsonOrNil(n sql.NullString) any {
	if !n.Valid {
		return nil
	}
	return decodeJSONFallback(n.String)
}
