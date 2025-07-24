package detector

import (
	"testing"
	"time"
)

func TestQpsTierClassifier_Basic(t *testing.T) {
	tier := NewQpsTierClassifier([]int{10, 20, 30}) // 每秒限速 10、10、10（增量）
	counts := make([]int, 4)                        // 三个级别 + 超限

	start := time.Now()
	for time.Since(start) < time.Second {
		level := tier.Classify()
		counts[level]++
		time.Sleep(5 * time.Millisecond) // 控制节奏避免太快
	}

	total := 0
	for i, c := range counts {
		total += c
		t.Logf("Level %d: %d", i, c)
	}

	if total == 0 {
		t.Fatal("No requests classified")
	}
}

func TestQpsTierClassifier_EmptyPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Expected panic on empty tier")
		}
	}()
	NewQpsTierClassifier([]int{})
}

func TestQpsTierClassifier_UnorderedTiers(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Expected panic on unordered tiers")
		}
	}()
	NewQpsTierClassifier([]int{100, 100, 50})
}

func TestQpsTierClassifier_RateLimiting(t *testing.T) {
	tier := NewQpsTierClassifier([]int{5, 10})
	counts := make([]int, 3)

	for i := 0; i < 50; i++ {
		level := tier.Classify()
		counts[level]++
		time.Sleep(10 * time.Millisecond)
	}

	t.Logf("Level counts: %+v", counts)
	if counts[0] == 0 {
		t.Error("Expected some requests in level 0")
	}
	if counts[2] == 0 {
		t.Error("Expected some requests exceeding all levels")
	}
}

func TestQpsTierClassifier_Concurrency(t *testing.T) {
	tier := NewQpsTierClassifier([]int{50, 100})
	results := make(chan int, 200)

	for i := 0; i < 200; i++ {
		go func() {
			level := tier.Classify()
			results <- level
		}()
	}

	time.Sleep(time.Second)

	counts := make([]int, 3)
	for i := 0; i < 200; i++ {
		level := <-results
		counts[level]++
	}

	t.Logf("Concurrent level counts: %+v", counts)
	if counts[2] == 0 {
		t.Error("Expected some requests to exceed all levels under concurrency")
	}
}
