package llmreview

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// AIReviewIdempotencyKey 由 (submission, revision, prompt, prompt_version) 派生 AI 审核任务的幂等键。
//
// 入队侧(api)写这个 key、消费侧(ai-worker)用同一个 key 做去重校验,两边必须算出**完全相同**的值,
// 否则幂等保证会静默失效(重复处理 / 重试风暴)。所以这里收口成唯一实现,两个 binary 都调它,
// 杜绝任何一边私改格式导致漂移。
func AIReviewIdempotencyKey(submissionID uint64, revisionID uint64, promptID uint64, promptVersion int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d", submissionID, revisionID, promptID, promptVersion)))
	return hex.EncodeToString(sum[:])
}
