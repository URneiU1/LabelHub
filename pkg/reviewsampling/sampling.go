package reviewsampling

import (
	"fmt"
	"hash/fnv"
)

// ShouldReview deterministically assigns a task item submission to the human
// review sample. Stable assignment avoids changing the decision on retries.
func ShouldReview(taskID uint64, itemID uint64, labelerID uint64, percentage int) bool {
	if percentage <= 0 {
		return false
	}
	if percentage >= 100 {
		return true
	}
	hash := fnv.New64a()
	_, _ = fmt.Fprintf(hash, "%d:%d:%d", taskID, itemID, labelerID)
	return int(hash.Sum64()%100) < percentage
}
