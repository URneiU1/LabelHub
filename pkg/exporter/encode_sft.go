package exporter

import (
	"encoding/json"
	"io"
)

// sftMessage / sftRecord 是 OpenAI Chat 微调(SFT)样本的标准结构:
// 每条 approved 提交映射成一对 user/assistant 消息,可直接喂 OpenAI fine-tuning。
type sftMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type sftRecord struct {
	Messages []sftMessage   `json:"messages"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// sftPromptKeys / sftCompletionKeys 是只读候选键表(包内私有,不重新赋值):从结构化
// payload(题目)/answer(标注)里按优先级提取一段文本作为 user / assistant content,
// 命中候选键则取该字段,否则整体紧凑 JSON。corrected_answer 排在前面:qa_quality 里它是
// 人工修正后的标准答案,正是 SFT 想要的目标。注:completion 候选里的 "answer" 指 answer
// 对象内部名为 answer 的子字段(如 {"answer":"..."}),与顶层 answer 字段本身无关。
var sftPromptKeys = []string{"prompt", "question", "input", "instruction", "text"}
var sftCompletionKeys = []string{"completion", "output", "corrected_answer", "answer", "response", "summary", "text"}

// EncodeSFT 输出 OpenAI Chat 微调 JSONL:逐行 {"messages":[{user},{assistant}],"metadata":{...}}。
// user content 取题目 payload、assistant content 取标注 answer(均按候选键提取、回退整体 JSON),
// metadata 携带 submission/item 标识 + 质量溯源(含审核时 AI 分数 / 人工结论),供下游训练时按质量过滤。
func EncodeSFT(w io.Writer, cols []Column, rows []Row) (int, error) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	n := 0
	for _, row := range rows {
		rec := sftRecord{
			Messages: []sftMessage{
				{Role: "user", Content: extractText(pick(row, "payload"), sftPromptKeys)},
				{Role: "assistant", Content: extractText(pick(row, "answer"), sftCompletionKeys)},
			},
			Metadata: provenanceMeta(row),
		}
		if err := enc.Encode(rec); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// extractText 从一个值里取一段文本:字符串直出;map 按候选键顺序取第一个非空标量;
// 都没命中(或值是数组/嵌套结构)时,对整体做紧凑 JSON 序列化(关闭 HTML 转义)。
func extractText(v any, keys []string) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case map[string]any:
		for _, k := range keys {
			if inner, ok := t[k]; ok {
				if s := stringifyCell(inner); s != "" {
					return s
				}
			}
		}
		return stringifyCell(v)
	default:
		return stringifyCell(v)
	}
}

// provenanceMeta 收集一行的标识 + 质量溯源元数据(键缺失则不写;map 输出顺序由 Go encoding/json
// 的字母序实现行为决定,稳定可测)。
// 质量字段仅在导出勾选"含审核记录"时存在,可供下游按 AI 分数 / 人工结论筛选训练样本。
func provenanceMeta(row Row) map[string]any {
	meta := map[string]any{}
	for _, k := range []string{"submission_id", "item_id", "external_id"} {
		if v := pick(row, k); v != nil {
			meta[k] = v
		}
	}
	if v := pick(row, "ai_review.overall_score"); v != nil {
		meta["ai_overall_score"] = v
	}
	if v := pick(row, "human_review.verdict"); v != nil {
		meta["human_verdict"] = v
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}
