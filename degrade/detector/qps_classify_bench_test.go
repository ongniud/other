package detector

import (
	"testing"

	"golang.org/x/time/rate"
)

func BenchmarkQpsTierClassifier_Serial(b *testing.B) {
	classifier := NewQpsTierClassifier([]int{1000, 2000, 3000}) // 较大限额，避免限流

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifier.Classify()
	}
}

func BenchmarkQpsTierClassifier_Parallel_1CPU(b *testing.B) {
	classifier := NewQpsTierClassifier([]int{1000, 2000, 3000})
	b.SetParallelism(1)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			classifier.Classify()
		}
	})
}

func BenchmarkQpsTierClassifier_Parallel_4CPU(b *testing.B) {
	classifier := NewQpsTierClassifier([]int{1000, 2000, 3000})
	b.SetParallelism(4)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			classifier.Classify()
		}
	})
}

func BenchmarkQpsTierClassifier_HighContention(b *testing.B) {
	classifier := NewQpsTierClassifier([]int{10, 20, 30}) // 非常小的限额，容易触发锁竞争
	b.SetParallelism(4)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			classifier.Classify()
		}
	})
}

func benchmarkClassifier(levels []int, b *testing.B) {
	classifier := NewQpsTierClassifier(levels)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifier.Classify()
	}
}

func BenchmarkClassifier_Levels_3(b *testing.B) {
	benchmarkClassifier([]int{1000, 2000, 3000}, b)
}

func BenchmarkClassifier_Levels_5(b *testing.B) {
	benchmarkClassifier([]int{500, 1000, 2000, 4000, 8000}, b)
}

func BenchmarkClassifier_Levels_10(b *testing.B) {
	benchmarkClassifier([]int{100, 200, 400, 800, 1600, 3200, 6400, 12800, 25600, 51200}, b)
}

func BenchmarkClassifier_Levels_20(b *testing.B) {
	levels := make([]int, 20)
	base := 100
	for i := range levels {
		levels[i] = base << i // 100, 200, 400, ..., 52428800
	}
	benchmarkClassifier(levels, b)
}

func BenchmarkNativeRateLimiter(b *testing.B) {
	limiter := rate.NewLimiter(rate.Limit(1000000), 1000000) // 高 QPS，避免速率瓶颈
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow()
	}
}
