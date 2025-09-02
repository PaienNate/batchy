package test

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	batcher "github.com/PaienNate/batchy"
)

// MemorySnapshot 内存快照结构
type MemorySnapshot struct {
	Timestamp    time.Time
	Alloc        uint64 // 当前分配的字节数
	TotalAlloc   uint64 // 累计分配的字节数
	Sys          uint64 // 从系统获得的字节数
	Lookups      uint64 // 指针查找次数
	Mallocs      uint64 // 分配次数
	Frees        uint64 // 释放次数
	HeapAlloc    uint64 // 堆分配字节数
	HeapSys      uint64 // 堆系统字节数
	HeapIdle     uint64 // 堆空闲字节数
	HeapInuse    uint64 // 堆使用字节数
	HeapReleased uint64 // 释放给OS的堆字节数
	HeapObjects  uint64 // 堆对象数量
	StackInuse   uint64 // 栈使用字节数
	StackSys     uint64 // 栈系统字节数
	MSpanInuse   uint64 // MSpan使用字节数
	MSpanSys     uint64 // MSpan系统字节数
	MCacheInuse  uint64 // MCache使用字节数
	MCacheSys    uint64 // MCache系统字节数
	BuckHashSys  uint64 // 分析哈希表字节数
	GCSys        uint64 // GC系统字节数
	OtherSys     uint64 // 其他系统字节数
	NextGC       uint64 // 下次GC目标堆大小
	LastGC       uint64 // 上次GC时间
	PauseTotalNs uint64 // GC暂停总时间
	PauseNs      [256]uint64 // 最近GC暂停时间
	PauseEnd     [256]uint64 // 最近GC结束时间
	NumGC        uint32 // GC次数
	NumForcedGC  uint32 // 强制GC次数
	GCCPUFraction float64 // GC CPU占用比例
	Goroutines   int    // Goroutine数量
}

// takeMemorySnapshot 获取详细的内存快照
func takeMemorySnapshot() *MemorySnapshot {
	// 强制垃圾回收以获得准确的内存状态
	runtime.GC()
	runtime.GC() // 执行两次确保完全清理
	time.Sleep(10 * time.Millisecond) // 等待GC完成
	
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	return &MemorySnapshot{
		Timestamp:     time.Now(),
		Alloc:         m.Alloc,
		TotalAlloc:    m.TotalAlloc,
		Sys:           m.Sys,
		Lookups:       m.Lookups,
		Mallocs:       m.Mallocs,
		Frees:         m.Frees,
		HeapAlloc:     m.HeapAlloc,
		HeapSys:       m.HeapSys,
		HeapIdle:      m.HeapIdle,
		HeapInuse:     m.HeapInuse,
		HeapReleased:  m.HeapReleased,
		HeapObjects:   m.HeapObjects,
		StackInuse:    m.StackInuse,
		StackSys:      m.StackSys,
		MSpanInuse:    m.MSpanInuse,
		MSpanSys:      m.MSpanSys,
		MCacheInuse:   m.MCacheInuse,
		MCacheSys:     m.MCacheSys,
		BuckHashSys:   m.BuckHashSys,
		GCSys:         m.GCSys,
		OtherSys:      m.OtherSys,
		NextGC:        m.NextGC,
		LastGC:        m.LastGC,
		PauseTotalNs:  m.PauseTotalNs,
		PauseNs:       m.PauseNs,
		PauseEnd:      m.PauseEnd,
		NumGC:         m.NumGC,
		NumForcedGC:   m.NumForcedGC,
		GCCPUFraction: m.GCCPUFraction,
		Goroutines:    runtime.NumGoroutine(),
	}
}

// MemoryDiff 计算两个内存快照的差异
type MemoryDiff struct {
	Duration      time.Duration
	AllocDiff     int64
	TotalAllocDiff int64
	SysDiff       int64
	MallocsDiff   int64
	FreesDiff     int64
	HeapAllocDiff int64
	HeapObjectsDiff int64
	GoroutinesDiff int
	GCCountDiff   int32
	GCTimeDiff    int64
}

// calculateMemoryDiff 计算内存差异
func calculateMemoryDiff(before, after *MemorySnapshot) *MemoryDiff {
	return &MemoryDiff{
		Duration:        after.Timestamp.Sub(before.Timestamp),
		AllocDiff:       int64(after.Alloc) - int64(before.Alloc),
		TotalAllocDiff:  int64(after.TotalAlloc) - int64(before.TotalAlloc),
		SysDiff:         int64(after.Sys) - int64(before.Sys),
		MallocsDiff:     int64(after.Mallocs) - int64(before.Mallocs),
		FreesDiff:       int64(after.Frees) - int64(before.Frees),
		HeapAllocDiff:   int64(after.HeapAlloc) - int64(before.HeapAlloc),
		HeapObjectsDiff: int64(after.HeapObjects) - int64(before.HeapObjects),
		GoroutinesDiff:  after.Goroutines - before.Goroutines,
		GCCountDiff:     int32(after.NumGC) - int32(before.NumGC),
		GCTimeDiff:      int64(after.PauseTotalNs) - int64(before.PauseTotalNs),
	}
}

// isMemoryLeakDetected 根据行业标准检测内存泄漏
func isMemoryLeakDetected(diff *MemoryDiff, itemsProcessed int64) (bool, string) {
	// 行业标准内存泄漏检测规则
	
	// 1. 检查未释放的对象数量
	unreleasedObjects := diff.MallocsDiff - diff.FreesDiff
	if unreleasedObjects > itemsProcessed/10 { // 超过处理项目10%的对象未释放
		return true, fmt.Sprintf("未释放对象过多: %d (处理项目: %d)", unreleasedObjects, itemsProcessed)
	}
	
	// 2. 检查堆内存增长率
	heapGrowthRate := float64(diff.HeapAllocDiff) / float64(itemsProcessed)
	if heapGrowthRate > 1024 { // 每个处理项目超过1KB的堆增长
		return true, fmt.Sprintf("堆内存增长率过高: %.2f bytes/item", heapGrowthRate)
	}
	
	// 3. 检查总分配内存与释放内存的比例
	if diff.MallocsDiff > 0 {
		freeRate := float64(diff.FreesDiff) / float64(diff.MallocsDiff)
		if freeRate < 0.95 { // 释放率低于95%
			return true, fmt.Sprintf("内存释放率过低: %.2f%% (分配: %d, 释放: %d)", 
				freeRate*100, diff.MallocsDiff, diff.FreesDiff)
		}
	}
	
	// 4. 检查堆对象数量增长
	if diff.HeapObjectsDiff > itemsProcessed/5 { // 堆对象增长超过处理项目的20%
		return true, fmt.Sprintf("堆对象增长过多: %d (处理项目: %d)", diff.HeapObjectsDiff, itemsProcessed)
	}
	
	// 5. 检查Goroutine泄漏
	if diff.GoroutinesDiff > 2 { // 允许少量Goroutine波动
		return true, fmt.Sprintf("Goroutine泄漏: 增加了 %d 个", diff.GoroutinesDiff)
	}
	
	return false, ""
}

// TestAdvancedMemoryLeak 高级内存泄漏检测测试
func TestAdvancedMemoryLeak(t *testing.T) {
	// 设置更严格的GC参数
	oldGCPercent := debug.SetGCPercent(10) // 更频繁的GC
	defer debug.SetGCPercent(oldGCPercent)
	
	// 预热阶段 - 让Go运行时稳定
	t.Log("开始预热阶段...")
	for i := 0; i < 3; i++ {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	}
	
	// 获取基线内存快照
	baseline := takeMemorySnapshot()
	t.Logf("基线内存快照: Alloc=%d KB, HeapObjects=%d, Goroutines=%d", 
		baseline.Alloc/1024, baseline.HeapObjects, baseline.Goroutines)
	
	// 执行多轮测试以检测累积内存泄漏
	const rounds = 5
	const itemsPerRound = 50000
	totalItemsProcessed := int64(0)
	
	for round := 0; round < rounds; round++ {
		t.Logf("\n=== 第 %d/%d 轮内存泄漏测试 ===", round+1, rounds)
		
		roundStart := takeMemorySnapshot()
		
		// 创建批处理器
		var processedInRound int64
		processor := func(items []string) []error {
			atomic.AddInt64(&processedInRound, int64(len(items)))
			// 模拟一些内存分配和释放
			for _, item := range items {
				_ = fmt.Sprintf("processed-%s", item) // 创建临时字符串
			}
			time.Sleep(time.Microsecond) // 模拟处理时间
			return nil
		}
		
		config := batcher.BatchConfig{
			BatchSize: 100,
			PoolSize:  8,
			QueueSize: 2000,
			Timeout:   50 * time.Millisecond,
			Ctx:       context.Background(),
		}
		
		b, err := batcher.NewChanBatcher[string](processor, config)
		if err != nil {
			t.Fatalf("创建批处理器失败: %v", err)
		}
		
		// 并发添加数据
		var wg sync.WaitGroup
		const numWorkers = 20
		itemsPerWorker := itemsPerRound / numWorkers
		
		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for i := 0; i < itemsPerWorker; i++ {
					item := fmt.Sprintf("round-%d-worker-%d-item-%d", round, workerID, i)
					err := b.Add(item)
					if err != nil {
						t.Errorf("添加数据失败: %v", err)
						return
					}
				}
			}(w)
		}
		
		wg.Wait()
		
		// 等待处理完成
		time.Sleep(500 * time.Millisecond)
		
		// 停止批处理器
		b.Stop()
		
		// 强制垃圾回收
		for i := 0; i < 5; i++ {
			runtime.GC()
			time.Sleep(20 * time.Millisecond)
		}
		
		roundEnd := takeMemorySnapshot()
		roundDiff := calculateMemoryDiff(roundStart, roundEnd)
		
		processedCount := atomic.LoadInt64(&processedInRound)
		totalItemsProcessed += processedCount
		
		t.Logf("第 %d 轮结果:", round+1)
		t.Logf("  处理项目: %d", processedCount)
		t.Logf("  内存分配: %d KB", roundDiff.AllocDiff/1024)
		t.Logf("  堆对象变化: %d", roundDiff.HeapObjectsDiff)
		t.Logf("  Goroutine变化: %d", roundDiff.GoroutinesDiff)
		t.Logf("  分配/释放: %d/%d", roundDiff.MallocsDiff, roundDiff.FreesDiff)
		
		// 检测本轮是否有内存泄漏
		if leaked, reason := isMemoryLeakDetected(roundDiff, processedCount); leaked {
			t.Errorf("第 %d 轮检测到内存泄漏: %s", round+1, reason)
		}
	}
	
	// 最终内存检查
	final := takeMemorySnapshot()
	totalDiff := calculateMemoryDiff(baseline, final)
	
	t.Logf("\n=== 最终内存泄漏分析 ===")
	t.Logf("总处理项目: %d", totalItemsProcessed)
	t.Logf("总测试时长: %v", totalDiff.Duration)
	t.Logf("内存分配增长: %d KB", totalDiff.AllocDiff/1024)
	t.Logf("系统内存增长: %d KB", totalDiff.SysDiff/1024)
	t.Logf("堆对象增长: %d", totalDiff.HeapObjectsDiff)
	t.Logf("Goroutine增长: %d", totalDiff.GoroutinesDiff)
	t.Logf("总分配/释放: %d/%d", totalDiff.MallocsDiff, totalDiff.FreesDiff)
	t.Logf("GC次数增加: %d", totalDiff.GCCountDiff)
	t.Logf("GC时间增加: %d ms", totalDiff.GCTimeDiff/1000000)
	
	// 最终内存泄漏检测
	if leaked, reason := isMemoryLeakDetected(totalDiff, totalItemsProcessed); leaked {
		t.Errorf("❌ 检测到内存泄漏: %s", reason)
	} else {
		t.Logf("✅ 内存泄漏检测通过")
	}
	
	// 计算内存效率指标
	memoryEfficiency := float64(totalDiff.AllocDiff) / float64(totalItemsProcessed)
	t.Logf("内存效率: %.2f bytes/item", memoryEfficiency)
	
	if memoryEfficiency > 512 { // 每个项目超过512字节认为效率低
		t.Errorf("❌ 内存效率过低: %.2f bytes/item", memoryEfficiency)
	} else {
		t.Logf("✅ 内存效率良好: %.2f bytes/item", memoryEfficiency)
	}
}

// TestMemoryLeakUnderStress 压力测试下的内存泄漏检测
func TestMemoryLeakUnderStress(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过压力测试")
	}
	
	t.Log("开始压力测试下的内存泄漏检测...")
	
	baseline := takeMemorySnapshot()
	
	// 高强度压力测试
	var totalProcessed int64
	processor := func(items []string) []error {
		atomic.AddInt64(&totalProcessed, int64(len(items)))
		// 模拟复杂处理逻辑
		for _, item := range items {
			// 创建一些临时对象
			data := make([]byte, 1024) // 1KB临时数据
			_ = fmt.Sprintf("%s-%x", item, data[:16])
		}
		return nil
	}
	
	config := batcher.BatchConfig{
		BatchSize: 200,
		PoolSize:  16,
		QueueSize: 5000,
		Timeout:   10 * time.Millisecond,
		Ctx:       context.Background(),
	}
	
	b, err := batcher.NewChanBatcher[string](processor, config)
	if err != nil {
		t.Fatalf("创建批处理器失败: %v", err)
	}
	
	// 启动多个生产者
	var wg sync.WaitGroup
	const producers = 50
	const itemsPerProducer = 10000
	
	start := time.Now()
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				item := fmt.Sprintf("stress-producer-%d-item-%d", producerID, i)
				err := b.Add(item)
				if err != nil {
					return // 批处理器可能已停止
				}
			}
		}(p)
	}
	
	wg.Wait()
	duration := time.Since(start)
	
	// 等待处理完成
	time.Sleep(2 * time.Second)
	
	b.Stop()
	
	// 强制垃圾回收
	for i := 0; i < 10; i++ {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	}
	
	final := takeMemorySnapshot()
	diff := calculateMemoryDiff(baseline, final)
	
	processedCount := atomic.LoadInt64(&totalProcessed)
	throughput := float64(processedCount) / duration.Seconds()
	
	t.Logf("压力测试结果:")
	t.Logf("  处理项目: %d", processedCount)
	t.Logf("  测试时长: %v", duration)
	t.Logf("  吞吐量: %.2f items/sec", throughput)
	t.Logf("  内存增长: %d KB", diff.AllocDiff/1024)
	t.Logf("  堆对象增长: %d", diff.HeapObjectsDiff)
	t.Logf("  Goroutine增长: %d", diff.GoroutinesDiff)
	
	// 压力测试下的内存泄漏检测
	if leaked, reason := isMemoryLeakDetected(diff, processedCount); leaked {
		t.Errorf("❌ 压力测试下检测到内存泄漏: %s", reason)
	} else {
		t.Logf("✅ 压力测试下无内存泄漏")
	}
}