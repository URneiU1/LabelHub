package handler

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"labelhub-api/internal/httpx"
	"labelhub-api/internal/middleware"
	"labelhub-api/internal/model"
)

// StatsHandler 提供 Owner 看板统计端点。
type StatsHandler struct {
	db *gorm.DB
}

func NewStatsHandler(db *gorm.DB) StatsHandler {
	return StatsHandler{db: db}
}

func (h StatsHandler) Register(api gin.IRouter) {
	api.GET("/tasks/:taskId/stats", middleware.RequireRoles("owner", "admin"), h.TaskStats)
}

type dimAverage struct {
	Name string  `json:"name"`
	Avg  float64 `json:"avg"`
}

// verdictPair 是一条已同时有 AI 判定与人工终判的提交,用于一致率与混淆矩阵。
type verdictPair struct {
	AIVerdict    string `gorm:"column:ai_verdict"`
	HumanVerdict string `gorm:"column:human_verdict"`
}

// confusionCell 是混淆矩阵的一格:某个 AI 判定 × 某个人工终判 的提交数。
type confusionCell struct {
	AI    string `json:"ai"`
	Human string `json:"human"`
	Count int    `json:"count"`
}

// scoreBucket 是 AI 总分的一个分桶区间的提交数。
type scoreBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// trendPoint 是某一天完成(approved/rejected)的提交数。
type trendPoint struct {
	Day   string `json:"day" gorm:"column:day"`
	Count int    `json:"count" gorm:"column:count"`
}

type taskStatsResponse struct {
	Progress struct {
		Total    int `json:"total"`
		Finished int `json:"finished"`
	} `json:"progress"`
	StatusBreakdown map[string]int `json:"statusBreakdown"`
	PassRate        float64        `json:"passRate"`
	AIvsHuman       struct {
		Compared int     `json:"compared"`
		Disagree int     `json:"disagree"`
		Rate     float64 `json:"rate"`
	} `json:"aiVsHuman"`
	DimensionAverages []dimAverage    `json:"dimensionAverages"`
	Confusion         []confusionCell `json:"confusion"`
	ScoreBuckets      []scoreBucket   `json:"scoreBuckets"`
	CompletionTrend   []trendPoint    `json:"completionTrend"`
}

func (h StatsHandler) TaskStats(c *gin.Context) {
	task, ok := loadOwnedTask(h.db, c)
	if !ok {
		return
	}
	var resp taskStatsResponse
	resp.Progress.Total = task.TotalItems
	resp.Progress.Finished = task.FinishedItems

	var statusRows []struct {
		Status string
		Count  int
	}
	if err := h.db.Model(&model.Submission{}).
		Select("status, COUNT(*) AS count").
		Where("task_id = ?", task.ID).
		Group("status").
		Scan(&statusRows).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load status breakdown")
		return
	}
	resp.StatusBreakdown = make(map[string]int, len(statusRows))
	for _, row := range statusRows {
		resp.StatusBreakdown[row.Status] = row.Count
	}
	approved := resp.StatusBreakdown["approved"]
	rejected := resp.StatusBreakdown["rejected"]
	if approved+rejected > 0 {
		resp.PassRate = float64(approved) / float64(approved+rejected)
	}

	var pairs []verdictPair
	if err := h.db.Model(&model.Submission{}).
		Select("ai_verdict, human_verdict").
		Where("task_id = ? AND ai_verdict IS NOT NULL AND human_verdict IS NOT NULL", task.ID).
		Scan(&pairs).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load ai/human comparison")
		return
	}
	resp.AIvsHuman.Compared = len(pairs)
	for _, p := range pairs {
		if !aiHumanAgree(p.AIVerdict, p.HumanVerdict) {
			resp.AIvsHuman.Disagree++
		}
	}
	if resp.AIvsHuman.Compared > 0 {
		resp.AIvsHuman.Rate = float64(resp.AIvsHuman.Disagree) / float64(resp.AIvsHuman.Compared)
	}
	resp.Confusion = buildConfusion(pairs)

	var dimRows []struct {
		Dimensions string `gorm:"column:dimensions"`
	}
	if err := h.db.Table("ai_reviews").
		Select("ai_reviews.dimensions").
		Joins("JOIN submissions ON submissions.id = ai_reviews.submission_id").
		Where("submissions.task_id = ? AND submissions.status = 'approved' AND ai_reviews.revision_id = submissions.current_revision_id AND ai_reviews.status = 'succeeded' AND ai_reviews.dimensions IS NOT NULL", task.ID).
		Scan(&dimRows).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load dimensions")
		return
	}
	dims := make([]string, 0, len(dimRows))
	for _, d := range dimRows {
		dims = append(dims, d.Dimensions)
	}
	resp.DimensionAverages = aggregateDimensions(dims)

	var scoreRows []struct {
		AIScore float64 `gorm:"column:ai_score"`
	}
	if err := h.db.Model(&model.Submission{}).
		Select("ai_score").
		Where("task_id = ? AND ai_score IS NOT NULL", task.ID).
		Scan(&scoreRows).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load ai scores")
		return
	}
	scores := make([]float64, 0, len(scoreRows))
	for _, r := range scoreRows {
		scores = append(scores, r.AIScore)
	}
	resp.ScoreBuckets = bucketScores(scores)

	var trendRows []trendPoint
	if err := h.db.Model(&model.Submission{}).
		Select("DATE(updated_at) AS day, COUNT(*) AS count").
		Where("task_id = ? AND status IN ?", task.ID, []string{"approved", "rejected"}).
		Group("DATE(updated_at)").
		Order("day").
		Scan(&trendRows).Error; err != nil {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load completion trend")
		return
	}
	resp.CompletionTrend = trendRows

	httpx.OK(c, resp)
}

// aiHumanAgree 把 AI 判定映射到人工判定:pass↔approve、reject↔reject 视为一致;
// 其它(含 uncertain)视为不一致。
func aiHumanAgree(ai, human string) bool {
	switch ai {
	case "pass":
		return human == "approve"
	case "reject":
		return human == "reject"
	default:
		return false
	}
}

// aggregateDimensions 按维度名对 ai_reviews.dimensions(JSON 数组)求均分;
// 坏 JSON 跳过不崩;输出按维度名排序保证稳定。纯函数,单独 TDD。
func aggregateDimensions(rawDims []string) []dimAverage {
	type acc struct {
		sum   float64
		count int
	}
	totals := map[string]*acc{}
	for _, raw := range rawDims {
		var items []struct {
			Name  string  `json:"name"`
			Score float64 `json:"score"`
		}
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			continue
		}
		for _, item := range items {
			if item.Name == "" {
				continue
			}
			a, ok := totals[item.Name]
			if !ok {
				a = &acc{}
				totals[item.Name] = a
			}
			a.sum += item.Score
			a.count++
		}
	}
	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]dimAverage, 0, len(names))
	for _, name := range names {
		a := totals[name]
		if a.count == 0 {
			continue
		}
		out = append(out, dimAverage{Name: name, Avg: a.sum / float64(a.count)})
	}
	return out
}

// buildConfusion 把 (AI 判定, 人工终判) 对聚合成混淆矩阵格,按 ai、human 排序保证稳定。
// 不假定判定取值,出现哪些组合就输出哪些(前端按固定坐标轴查表,缺失补 0)。纯函数。
func buildConfusion(pairs []verdictPair) []confusionCell {
	counts := map[[2]string]int{}
	for _, p := range pairs {
		counts[[2]string{p.AIVerdict, p.HumanVerdict}]++
	}
	out := make([]confusionCell, 0, len(counts))
	for key, count := range counts {
		out = append(out, confusionCell{AI: key[0], Human: key[1], Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AI != out[j].AI {
			return out[i].AI < out[j].AI
		}
		return out[i].Human < out[j].Human
	})
	return out
}

// bucketScores 把 AI 总分(0-100)分到固定区间。区间左闭右开,最后一档含 100。纯函数。
func bucketScores(scores []float64) []scoreBucket {
	defs := []struct {
		label  string
		lo, hi float64
	}{
		{"<60", 0, 60},
		{"60-70", 60, 70},
		{"70-80", 70, 80},
		{"80-90", 80, 90},
		{"90-100", 90, 100.0001},
	}
	out := make([]scoreBucket, len(defs))
	for i, d := range defs {
		out[i] = scoreBucket{Label: d.label}
	}
	for _, s := range scores {
		for i, d := range defs {
			if s >= d.lo && s < d.hi {
				out[i].Count++
				break
			}
		}
	}
	return out
}
