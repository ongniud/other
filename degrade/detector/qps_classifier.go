package detector

import (
	"golang.org/x/time/rate"
	"time"
)

// QpsTierClassifier classifies requests based on QPS tiers.
type QpsTierClassifier struct {
	limiters  []*rate.Limiter // Rate limiters for each QPS tier
	tiers     []int           // QPS thresholds
	createdAt time.Time
}

// NewQpsTierClassifier initializes a classifier with given QPS tiers.
func NewQpsTierClassifier(tiers []int) *QpsTierClassifier {
	if len(tiers) == 0 {
		panic("tiers cannot be empty")
	}
	for i := 1; i < len(tiers); i++ {
		if tiers[i] <= tiers[i-1] {
			panic("tiers must be in strictly ascending order")
		}
	}

	deltas := make([]int, len(tiers))
	deltas[0] = tiers[0]
	for i := 1; i < len(tiers); i++ {
		deltas[i] = tiers[i] - tiers[i-1]
	}

	limiters := make([]*rate.Limiter, len(deltas))
	for i, delta := range deltas {
		limiters[i] = rate.NewLimiter(rate.Limit(delta), delta)
	}

	return &QpsTierClassifier{
		limiters:  limiters,
		tiers:     tiers,
		createdAt: time.Now(),
	}
}

// Classify returns the tier level for a request based on QPS.
func (qc *QpsTierClassifier) Classify() int {
	for level, limiter := range qc.limiters {
		if limiter.Allow() {
			return level
		}
	}
	return len(qc.limiters) // Request exceeds all limits
}
