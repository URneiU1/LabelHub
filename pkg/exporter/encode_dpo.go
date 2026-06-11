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

// EncodeDPO 针对 preference_compare 任务输出 DPO 偏好对 JSONL。
// payload 含 prompt / response_a / response_b,标注 answer.preferred ∈ {A,B,tie}:
// preferred=A → chosen=response_a、rejected=response_b;=B → 反之;
// tie / 缺失 / 任一回答为空 → 跳过(无有效偏好信号,绝不产出残缺样本)。
// 返回真实产出的样本数(已跳过的 tie 不计入),如实反映可用偏好对数量。
func EncodeDPO(w io.Writer, cols []Column, rows []Row) (int, error) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	n := 0
	for _, row := range rows {
		payload, _ := pick(row, "payload").(map[string]any)
		answer, _ := pick(row, "answer").(map[string]any)
		if payload == nil || answer == nil {
			continue
		}

		// preference_compare 是约定 schema(response_a/b、model_a/b 为固定字段),
		// 故直接按键取值,不像 SFT 那样做候选键回退。
		respA := stringifyCell(payload["response_a"])
		respB := stringifyCell(payload["response_b"])
		modelA := stringifyCell(payload["model_a"])
		modelB := stringifyCell(payload["model_b"])

		var chosen, rejected, chosenModel, rejectedModel string
		switch normPreferred(answer["preferred"]) {
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
			Prompt:   stringifyCell(payload["prompt"]),
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
