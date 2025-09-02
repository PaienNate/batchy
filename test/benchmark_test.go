package test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	batcher "github.com/PaienNate/batchy"
)

// BenchmarkChanBatcher 批处理器性能基准测试
func BenchmarkChanBatcher(b *testing.B) {
	processor := func(items []string) []error {
		// 模拟轻量级处理
		return nil
	}
	
	config := batcher.BatchConfig{
		BatchSize: 1000,
		PoolSize:  10,
		QueueSize: 10000,
		Timeout:   100 * time.Millisecond,
		Ctx:       context.Background(),
	}
	
	batcherInstance, err := batcher.NewChanBatcher[string](processor, config)
	if err != nil {
		b.Fatalf("创建批处理器失败: %v", err)
	}
	defer batcherInstance.Stop()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			err := batcherInstance.Add(fmt.Sprintf("item-%d", i))
			if err != nil {
				b.Errorf("添加数据失败: %v", err)
			}
			i++
		}
	})
}

// BenchmarkDynamicBatching 动态批处理性能测试
func BenchmarkDynamicBatching(b *testing.B) {
	processor := func(items []string) []error {
		return nil
	}
	
	config := batcher.BatchConfig{
		BatchSize:         500,
		PoolSize:          8,
		QueueSize:         5000,
		Timeout:           50 * time.Millisecond,
		Ctx:               context.Background(),
		DynamicBatching:   true,
		MinBatchSize:      100,
		MaxBatchSize:      2000,
		AdaptiveThreshold: 25 * time.Millisecond,
	}
	
	batcherInstance, err := batcher.NewChanBatcher[string](processor, config)
	if err != nil {
		b.Fatalf("创建动态批处理器失败: %v", err)
	}
	defer batcherInstance.Stop()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			err := batcherInstance.Add(fmt.Sprintf("dynamic-item-%d", i))
			if err != nil {
				b.Errorf("添加数据失败: %v", err)
			}
			i++
		}
	})
}

// BenchmarkHighThroughput 高吞吐量性能测试
func BenchmarkHighThroughput(b *testing.B) {
	var processedCount int64
	
	processor := func(items []string) []error {
		atomic.AddInt64(&processedCount, int64(len(items)))
		return nil
	}
	
	config := batcher.BatchConfig{
		BatchSize: 2000,
		PoolSize:  20,
		QueueSize: 20000,
		Timeout:   200 * time.Millisecond,
		Ctx:       context.Background(),
	}
	
	batcherInstance, err := batcher.NewChanBatcher[string](processor, config)
	if err != nil {
		b.Fatalf("创建高吞吐量批处理器失败: %v", err)
	}
	defer batcherInstance.Stop()
	
	b.ResetTimer()
	start := time.Now()
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			err := batcherInstance.Add(fmt.Sprintf("high-throughput-item-%d", i))
			if err != nil {
				b.Errorf("添加数据失败: %v", err)
			}
			i++
		}
	})
	
	duration := time.Since(start)
	processed := atomic.LoadInt64(&processedCount)
	b.Logf("高吞吐量测试: 处理了 %d 项，耗时 %v，吞吐量 %.2f items/sec", 
		processed, duration, float64(processed)/duration.Seconds())
}

// TestPerformanceReport 生成详细的性能分析报告
func TestPerformanceReport(t *testing.T) {
	t.Log("=== 批处理系统性能分析报告 ===")
	
	// 测试不同配置下的性能
	configs := []struct {
		name      string
		batchSize int
		poolSize  int
		queueSize int
		timeout   time.Duration
		dynamic   bool
	}{
		{"小批量", 100, 5, 1000, 50 * time.Millisecond, false},
		{"中批量", 500, 10, 5000, 100 * time.Millisecond, false},
		{"大批量", 2000, 20, 20000, 200 * time.Millisecond, false},
		{"动态批量", 1000, 15, 10000, 100 * time.Millisecond, true},
	}
	
	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			var processedCount int64
			var batchCount int64
			
			processor := func(items []string) []error {
				atomic.AddInt64(&processedCount, int64(len(items)))
				atomic.AddInt64(&batchCount, 1)
				return nil
			}
			
			config := batcher.BatchConfig{
				BatchSize: cfg.batchSize,
				PoolSize:  cfg.poolSize,
				QueueSize: cfg.queueSize,
				Timeout:   cfg.timeout,
				Ctx:       context.Background(),
			}
			
			if cfg.dynamic {
				config.DynamicBatching = true
				config.MinBatchSize = cfg.batchSize / 2
				config.MaxBatchSize = cfg.batchSize * 2
				config.AdaptiveThreshold = cfg.timeout / 2
			}
			
			b, err := batcher.NewChanBatcher[string](processor, config)
			if err != nil {
				t.Fatalf("创建批处理器失败: %v", err)
			}
			
			// 记录初始内存状态
			runtime.GC()
			var m1 runtime.MemStats
			runtime.ReadMemStats(&m1)
			
			start := time.Now()
			
			// 并发添加数据
			var wg sync.WaitGroup
			const totalItems = 100000
			const numProducers = 10
			itemsPerProducer := totalItems / numProducers
			
			for p := 0; p < numProducers; p++ {
				wg.Add(1)
				go func(producerID int) {
					defer wg.Done()
					for i := 0; i < itemsPerProducer; i++ {
						err := b.Add(fmt.Sprintf("%s-item-%d-%d", cfg.name, producerID, i))
						if err != nil {
							t.Errorf("添加数据失败: %v", err)
							return
						}
					}
				}(p)
			}
			
			wg.Wait()
			
			// 等待处理完成
			for atomic.LoadInt64(&processedCount) < totalItems {
				time.Sleep(10 * time.Millisecond)
			}
			
			duration := time.Since(start)
			b.Stop()
			
			// 记录最终内存状态
			runtime.GC()
			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)
			
			processed := atomic.LoadInt64(&processedCount)
			batches := atomic.LoadInt64(&batchCount)
			throughput := float64(processed) / duration.Seconds()
			avgBatchSize := float64(processed) / float64(batches)
			memoryUsed := int64(m2.Alloc) - int64(m1.Alloc)
			
			t.Logf("配置: %s", cfg.name)
			t.Logf("  批大小: %d, 工作池: %d, 队列: %d, 超时: %v", 
				cfg.batchSize, cfg.poolSize, cfg.queueSize, cfg.timeout)
			t.Logf("  处理项目: %d, 批次数: %d, 平均批大小: %.1f", 
				processed, batches, avgBatchSize)
			t.Logf("  处理时间: %v, 吞吐量: %.2f items/sec", duration, throughput)
			t.Logf("  内存使用: %d KB, GC次数: %d", 
				memoryUsed/1024, m2.NumGC-m1.NumGC)
			t.Logf("  每项内存: %.2f bytes/item", float64(memoryUsed)/float64(processed))
			t.Log("")
		})
	}
	
	t.Log("=== 性能分析报告完成 ===")
}

// TestSchedulingPolicyPerformance 调度策略性能对比
func TestSchedulingPolicyPerformance(t *testing.T) {
	t.Log("=== 调度策略性能对比 ===")
	
	policies := []struct {
		name   string
		policy batcher.SchedulingPolicy
	}{
		{"轮询调度", batcher.ROUND_ROBIN},
		{"顺序调度", batcher.ORDERED_SEQUENTIAL},
	}
	
	for _, p := range policies {
		t.Run(p.name, func(t *testing.T) {
			var processedCount int64
			
			processor := func(items []string) []error {
				atomic.AddInt64(&processedCount, int64(len(items)))
				// 模拟处理时间
				time.Sleep(1 * time.Millisecond)
				return nil
			}
			
			config := batcher.BatchConfig{
				BatchSize:        1000,
				PoolSize:         10,
				QueueSize:        10000,
				Timeout:          100 * time.Millisecond,
				Ctx:              context.Background(),
				SchedulingPolicy: p.policy,
			}
			
			b, err := batcher.NewChanBatcher[string](processor, config)
			if err != nil {
				t.Fatalf("创建批处理器失败: %v", err)
			}
			
			start := time.Now()
			
			// 添加测试数据
			const totalItems = 50000
			for i := 0; i < totalItems; i++ {
				err := b.Add(fmt.Sprintf("%s-item-%d", p.name, i))
				if err != nil {
					t.Errorf("添加数据失败: %v", err)
					break
				}
			}
			
			// 等待处理完成
			for atomic.LoadInt64(&processedCount) < totalItems {
				time.Sleep(10 * time.Millisecond)
			}
			
			duration := time.Since(start)
			b.Stop()
			
			processed := atomic.LoadInt64(&processedCount)
			throughput := float64(processed) / duration.Seconds()
			
			t.Logf("%s: 处理 %d 项，耗时 %v，吞吐量 %.2f items/sec", 
				p.name, processed, duration, throughput)
		})
	}
}