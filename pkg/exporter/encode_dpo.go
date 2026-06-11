package exporter

import (
	"encoding/json"
	"io"
	"strings"
)

// dpoRecord 是标准 DPO 偏好对 {prompt, chosen, rejected}:可直接喂 TRL DPOTrainer。
// metadata 携带标识 + 偏好强度 + 双方模型名,供下游按 margin 过滤或做模型胜率分析。
type dpoRecord struct {
	Prompt   string         `json:"prompt"`
	Chosen   string         `json:"chosen"`
	Rejected string         `json:"rejected"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// EncodeDPO 输出 DPO 偏好对 JSONL(无字段映射配置,走 preference_compare 约定),保持向后兼容;
// 真实导出路径走 EncodeDPOWithConfig。
func EncodeDPO(w io.Writer, cols []Column, rows []Row) (int, error) {
	return EncodeDPOWithConfig(w, rows, nil)
}

// EncodeDPOWithConfig 输出 DPO 偏好对 JSONL,字段映射可配置以适配任意 A/B 偏好任务:
// cfg 各字段非空时按点号路径取 prompt / 候选 A / 候选 B(相对 payload)与 preferred(相对 answer),
// 为空则回退 preference_compare 约定(prompt / response_a / response_b / preferred)。
// preferred=A → chosen=候选A、rejected=候选B;=B → 反之;tie / 缺失 / 任一候选为空 → 跳过(不产残缺样本)。
// model_a/model_b 是约定的可选元数据(不参与映射),缺失则省略。返回真实产出的偏好对数。
func EncodeDPOWithConfig(w io.Writer, rows []Row, cfg *DPOConfig) (int, error) {
	promptF, aF, bF, prefF := "prompt", "response_a", "response_b", "preferred"
	if cfg != nil {
		if cfg.PromptField != "" {
			promptF = cfg.PromptField
		}
		if cfg.CandidateAField != "" {
			aF = cfg.CandidateAField
		}
		if cfg.CandidateBField != "" {
			bF = cfg.CandidateBField
		}
		if cfg.PreferredField != "" {
			prefF = cfg.PreferredField
		}
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	n := 0
	for _, row := range rows {
		payload, _ := pick(row, "payload").(map[string]any)
		answer, _ := pick(row, "answer").(map[string]any)
		if payload == nil || answer == nil {
			continue
		}

		respA := stringifyCell(resolvePath(payload, aF))
		respB := stringifyCell(resolvePath(payload, bF))
		modelA := stringifyCell(payload["model_a"])
		modelB := stringifyCell(payload["model_b"])

		var chosen, rejected, chosenModel, rejectedModel string
		switch normPreferred(resolvePath(answer, prefF)) {
		case "A":
			chosen, rejected, chosenModel, rejectedModel = respA, respB, modelA, modelB
		case "B":
			chosen, rejected, chosenModel, rejectedModel = respB, respA, modelB, modelA
		default:
			continue // tie / 未知 → 无偏好信号,跳过
		}
		if chosen == "" || rejected == "" {
			continue // 任一候选缺失 → 无法构成有效偏好对
		}

		rec := dpoRecord{
			Prompt:   stringifyCell(resolvePath(payload, promptF)),
			Chosen:   chosen,
			Rejected: rejected,
			Metadata: dpoMeta(row, answer, chosenModel, rejectedModel),
		}
		if err := enc.Encode(rec); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// normPreferred 把 preferred 归一成 "A"/"B"/"tie"/"":大小写不敏感,非法值归 ""。
func normPreferred(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a":
		return "A"
	case "b":
		return "B"
	case "tie":
		return "tie"
	default:
		return ""
	}
}

func dpoMeta(row Row, answer map[string]any, chosenModel, rejectedModel string) map[string]any {
	meta := provenanceMeta(row)
	if meta == nil {
		meta = map[string]any{}
	}
	if v, ok := answer["margin"]; ok {
		if s := stringifyCell(v); s != "" {
			meta["margin"] = s
		}
	}
	if chosenModel != "" {
		meta["chosen_model"] = chosenModel
	}
	if rejectedModel != "" {
		meta["rejected_model"] = rejectedModel
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}
