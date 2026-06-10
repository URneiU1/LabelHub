package reviewsampling

import "testing"

func TestShouldReviewBoundariesAndStability(t *testing.T) {
	if ShouldReview(1, 2, 3, 0) {
		t.Fatal("0 percent must skip review")
	}
	if !ShouldReview(1, 2, 3, 100) {
		t.Fatal("100 percent must require review")
	}
	first := ShouldReview(7, 11, 5, 37)
	for range 5 {
		if got := ShouldReview(7, 11, 5, 37); got != first {
			t.Fatalf("stable assignment changed: got %v, want %v", got, first)
		}
	}
}
