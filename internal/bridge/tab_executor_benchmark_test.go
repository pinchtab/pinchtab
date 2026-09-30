package bridge

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
)

func BenchmarkTabExecutor_SequentialSameTab(b *testing.B) {
	te := bridgetabs.NewTabExecutor(4)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = te.Execute(ctx, "tab1", func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkTabExecutor_ParallelDifferentTabs(b *testing.B) {
	te := bridgetabs.NewTabExecutor(8)
	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tabID := fmt.Sprintf("tab%d", i%8)
			_ = te.Execute(ctx, tabID, func(ctx context.Context) error {
				return nil
			})
			i++
		}
	})
}

func BenchmarkTabExecutor_ParallelSameTab(b *testing.B) {
	te := bridgetabs.NewTabExecutor(8)
	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = te.Execute(ctx, "tab1", func(ctx context.Context) error {
				return nil
			})
		}
	})
}

func BenchmarkTabExecutor_WithWork(b *testing.B) {
	te := bridgetabs.NewTabExecutor(4)
	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tabID := fmt.Sprintf("tab%d", i%4)
			_ = te.Execute(ctx, tabID, func(ctx context.Context) error {
				// Simulate light work
				sum := 0
				for j := 0; j < 100; j++ {
					sum += j
				}
				_ = sum
				return nil
			})
			i++
		}
	})
}

func BenchmarkSequentialVsParallel(b *testing.B) {
	workDuration := time.Microsecond * 100

	b.Run("Sequential_4Tabs", func(b *testing.B) {
		te := bridgetabs.NewTabExecutor(1)
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < 4; j++ {
				_ = te.Execute(ctx, fmt.Sprintf("tab%d", j), func(ctx context.Context) error {
					time.Sleep(workDuration)
					return nil
				})
			}
		}
	})

	b.Run("Parallel_4Tabs", func(b *testing.B) {
		te := bridgetabs.NewTabExecutor(4)
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			for j := 0; j < 4; j++ {
				wg.Add(1)
				tabID := fmt.Sprintf("tab%d", j)
				go func() {
					defer wg.Done()
					_ = te.Execute(ctx, tabID, func(ctx context.Context) error {
						time.Sleep(workDuration)
						return nil
					})
				}()
			}
			wg.Wait()
		}
	})
}
