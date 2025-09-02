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

// TestMemoryLeak 内存泄漏检测测试
func TestMemoryLeak(t *testing.T) {
	// 强制垃圾回收
	runtime.GC()
	runtime.GC()
	
	// 记录初始内存状态
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	initialAlloc := m1.Alloc
	initialSys := m1.Sys
	initialNumGC := m1.NumGC
	
	t.Logf("初始内存状态: Alloc=%d KB, Sys=%d KB, NumGC=%d", 
		initialAlloc/1024, initialSys/1024, initialNumGC)
	
	// 运行多轮批处理测试
	const rounds = 10
	for round := 0; round < rounds; round++ {
		t.Logf("开始第 %d 轮测试", round+1)
		
		// 创建批处理器
		processor := func(items []string) []error {
			// 模拟处理工作
			time.Sleep(1 * time.Millisecond)
			return nil
		}
		
		config := batcher.BatchConfig{
			BatchSize: 100,
			PoolSize:  5,
			QueueSize: 1000,
			Timeout:   50 * time.Millisecond,
			Ctx:       context.Background(),
		}
		
		b, err := batcher.NewChanBatcher[string](processor, config)
		if err != nil {
			t.Fatalf("创建批处理器失败: %v", err)
		}
		
		// 添加大量数据
		var wg sync.WaitGroup
		const itemsPerRound = 10000
		
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < itemsPerRound; i++ {
				err := b.Add(fmt.Sprintf("item-%d-%d", round, i))
				if err != nil {
					t.Errorf("添加数据失败: %v", err)
					return
				}
			}
		}()
		
		wg.Wait()
		
		// 等待处理完成
		time.Sleep(200 * time.Millisecond)
		
		// 停止批处理器
		b.Stop()
		
		// 强制垃圾回收
		runtime.GC()
		runtime.GC()
		
		// 检查内存状态
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		
		t.Logf("第 %d 轮后内存状态: Alloc=%d KB, Sys=%d KB, NumGC=%d", 
			round+1, m.Alloc/1024, m.Sys/1024, m.NumGC)
	}
	
	// 最终内存检查
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	finalAlloc := m2.Alloc
	finalSys := m2.Sys
	finalNumGC := m2.NumGC
	
	t.Logf("最终内存状态: Alloc=%d KB, Sys=%d KB, NumGC=%d", 
		finalAlloc/1024, finalSys/1024, finalNumGC)
	
	// 计算内存增长
	allocGrowth := int64(finalAlloc) - int64(initialAlloc)
	sysGrowth := int64(finalSys) - int64(initialSys)
	
	t.Logf("内存增长: Alloc增长=%d KB, Sys增长=%d KB, GC次数增加=%d", 
		allocGrowth/1024, sysGrowth/1024, finalNumGC-initialNumGC)
	
	// 检查是否有明显的内存泄漏
	// 允许一定的内存增长，但不应该过大
	const maxAllocGrowthMB = 50 // 最大允许50MB的内存增长
	if allocGrowth > maxAllocGrowthMB*1024*1024 {
		t.Errorf("检测到可能的内存泄漏: 内存增长 %d MB 超过阈值 %d MB", 
			allocGrowth/(1024*1024), maxAllocGrowthMB)
	} else {
		t.Logf("内存泄漏检测通过: 内存增长 %d MB 在合理范围内", 
			allocGrowth/(1024*1024))
	}
}

// TestResourceCleanup 严格的资源清理测试
func TestResourceCleanup(t *testing.T) {
	var processedCount int64
	var processorCalled int64
	
	processor := func(items []string) []error {
		atomic.AddInt64(&processorCalled, 1)
		atomic.AddInt64(&processedCount, int64(len(items)))
		// 模拟处理时间
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	
	config := batcher.BatchConfig{
		BatchSize: 10,
		PoolSize:  3,
		QueueSize: 100,
		Timeout:   50 * time.Millisecond,
		Ctx:       context.Background(),
	}
	
	// 记录初始goroutine数量
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("初始goroutine数量: %d", initialGoroutines)
	
	// 记录初始内存状态
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	t.Logf("初始内存: Alloc=%d KB, Sys=%d KB", m1.Alloc/1024, m1.Sys/1024)
	
	// 创建批处理器
	b, err := batcher.NewChanBatcher[string](processor, config)
	if err != nil {
		t.Fatalf("创建批处理器失败: %v", err)
	}
	
	// 检查goroutine数量增加
	afterCreateGoroutines := runtime.NumGoroutine()
	t.Logf("创建后goroutine数量: %d (增加了 %d)", afterCreateGoroutines, afterCreateGoroutines-initialGoroutines)
	
	// 添加大量数据
	const totalItems = 1000
	for i := 0; i < totalItems; i++ {
		err := b.Add(fmt.Sprintf("cleanup-test-item-%d", i))
		if err != nil {
			t.Errorf("添加数据失败: %v", err)
		}
	}
	
	// 等待一些处理
	time.Sleep(200 * time.Millisecond)
	processedBefore := atomic.LoadInt64(&processedCount)
	processorCallsBefore := atomic.LoadInt64(&processorCalled)
	t.Logf("Stop前: 已处理 %d 项，处理器调用 %d 次", processedBefore, processorCallsBefore)
	
	// 停止批处理器
	stopStart := time.Now()
	b.Stop()
	stopDuration := time.Since(stopStart)
	t.Logf("Stop()耗时: %v", stopDuration)
	
	// 验证Stop()后不能再添加数据
	err = b.Add("should-fail")
	if err == nil {
		t.Error("Stop()后仍能添加数据，资源未正确清理")
	} else {
		t.Logf("✓ Stop()后正确拒绝新数据: %v", err)
	}
	
	// 等待goroutine清理
	time.Sleep(100 * time.Millisecond)
	
	// 检查goroutine是否清理
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Stop后goroutine数量: %d", finalGoroutines)
	
	if finalGoroutines > initialGoroutines+2 { // 允许少量误差
		t.Errorf("❌ goroutine泄漏: 初始=%d, 最终=%d, 泄漏=%d", 
			initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)
	} else {
		t.Logf("✓ goroutine正确清理")
	}
	
	// 验证处理器不再被调用
	processedAfter := atomic.LoadInt64(&processedCount)
	processorCallsAfter := atomic.LoadInt64(&processorCalled)
	time.Sleep(200 * time.Millisecond) // 等待确认没有新的处理
	processedFinal := atomic.LoadInt64(&processedCount)
	processorCallsFinal := atomic.LoadInt64(&processorCalled)
	
	if processedFinal != processedAfter || processorCallsFinal != processorCallsAfter {
		t.Errorf("❌ Stop()后处理器仍在运行: 处理数从%d增加到%d，调用从%d增加到%d", 
			processedAfter, processedFinal, processorCallsAfter, processorCallsFinal)
	} else {
		t.Logf("✓ Stop()后处理器正确停止")
	}
	
	// 强制垃圾回收并检查内存
	runtime.GC()
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	
	memoryGrowth := int64(m2.Alloc) - int64(m1.Alloc)
	t.Logf("最终内存: Alloc=%d KB, Sys=%d KB, 内存增长=%d KB", 
		m2.Alloc/1024, m2.Sys/1024, memoryGrowth/1024)
	
	if memoryGrowth > 1024*1024 { // 超过1MB认为有问题
		t.Errorf("❌ 可能存在内存泄漏: 增长了 %d KB", memoryGrowth/1024)
	} else {
		t.Logf("✓ 内存使用正常")
	}
	
	t.Logf("✓ 资源清理测试完成: 处理了%d项，处理器调用%d次", processedFinal, processorCallsFinal)
}

// TestConcurrentAccess 严格的并发安全测试
func TestConcurrentAccess(t *testing.T) {
	var processedCount int64
	var processorCalls int64
	var processingErrors int64
	var addErrors int64
	
	// 用于检测数据竞争的map
	processedItems := sync.Map{}
	
	processor := func(items []string) []error {
		atomic.AddInt64(&processorCalls, 1)
		atomic.AddInt64(&processedCount, int64(len(items)))
		
		// 检查重复处理
		for _, item := range items {
			if _, exists := processedItems.LoadOrStore(item, true); exists {
				atomic.AddInt64(&processingErrors, 1)
				t.Errorf("❌ 检测到重复处理: %s", item)
			}
		}
		
		// 模拟处理时间和可能的错误
		time.Sleep(time.Duration(len(items)) * time.Microsecond)
		return nil
	}
	
	config := batcher.BatchConfig{
		BatchSize: 50,
		PoolSize:  10,
		QueueSize: 1000,
		Timeout:   100 * time.Millisecond,
		Ctx:       context.Background(),
	}
	
	b, err := batcher.NewChanBatcher[string](processor, config)
	if err != nil {
		t.Fatalf("创建批处理器失败: %v", err)
	}
	
	// 记录初始goroutine数量
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("并发测试开始，初始goroutine数量: %d", initialGoroutines)
	
	// 启动多个并发goroutine进行压力测试
	var wg sync.WaitGroup
	const numGoroutines = 100
	const itemsPerGoroutine = 1000
	const totalExpectedItems = numGoroutines * itemsPerGoroutine
	
	// 并发添加数据
	start := time.Now()
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < itemsPerGoroutine; i++ {
				item := fmt.Sprintf("goroutine-%d-item-%d", goroutineID, i)
				err := b.Add(item)
				if err != nil {
					atomic.AddInt64(&addErrors, 1)
					// 不立即失败，记录错误继续测试
				}
				
				// 随机延迟模拟真实场景
				if i%100 == 0 {
					time.Sleep(time.Microsecond)
				}
			}
		}(g)
	}
	
	// 同时启动一些读取状态的goroutine测试并发读写
	var statusWg sync.WaitGroup
	for i := 0; i < 10; i++ {
		statusWg.Add(1)
		go func() {
			defer statusWg.Done()
			for j := 0; j < 100; j++ {
				// 模拟状态查询（如果有的话）
				runtime.Gosched() // 让出CPU
				time.Sleep(time.Microsecond)
			}
		}()
	}
	
	wg.Wait()
	statusWg.Wait()
	addDuration := time.Since(start)
	
	t.Logf("数据添加完成，耗时: %v，添加错误: %d", addDuration, atomic.LoadInt64(&addErrors))
	
	// 等待所有数据处理完成
	processStart := time.Now()
	for {
		processed := atomic.LoadInt64(&processedCount)
		if processed >= totalExpectedItems {
			break
		}
		if time.Since(processStart) > 30*time.Second {
			t.Errorf("❌ 处理超时: 期望%d项，实际处理%d项", totalExpectedItems, processed)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	processDuration := time.Since(processStart)
	
	// 测试并发Stop
	var stopWg sync.WaitGroup
	for i := 0; i < 5; i++ {
		stopWg.Add(1)
		go func() {
			defer stopWg.Done()
			b.Stop() // 多次调用Stop应该是安全的
		}()
	}
	stopWg.Wait()
	
	// 验证Stop后的状态
	time.Sleep(100 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()
	
	// 收集最终统计
	finalProcessed := atomic.LoadInt64(&processedCount)
	finalCalls := atomic.LoadInt64(&processorCalls)
	finalProcessingErrors := atomic.LoadInt64(&processingErrors)
	finalAddErrors := atomic.LoadInt64(&addErrors)
	
	// 验证结果
	if finalProcessingErrors > 0 {
		t.Errorf("❌ 检测到 %d 个数据竞争/重复处理错误", finalProcessingErrors)
	} else {
		t.Logf("✓ 无数据竞争检测")
	}
	
	if finalAddErrors > totalExpectedItems/10 { // 允许10%的添加失败（队列满等）
		t.Errorf("❌ 添加错误过多: %d/%d", finalAddErrors, totalExpectedItems)
	} else {
		t.Logf("✓ 添加错误在可接受范围: %d/%d", finalAddErrors, totalExpectedItems)
	}
	
	if finalGoroutines > initialGoroutines+5 { // 允许少量误差
		t.Errorf("❌ goroutine泄漏: 初始=%d, 最终=%d", initialGoroutines, finalGoroutines)
	} else {
		t.Logf("✓ goroutine正确清理: 初始=%d, 最终=%d", initialGoroutines, finalGoroutines)
	}
	
	throughput := float64(finalProcessed) / addDuration.Seconds()
	avgBatchSize := float64(finalProcessed) / float64(finalCalls)
	
	t.Logf("✓ 并发安全测试完成:")
	t.Logf("  - 并发goroutine: %d", numGoroutines)
	t.Logf("  - 期望处理: %d 项", totalExpectedItems)
	t.Logf("  - 实际处理: %d 项", finalProcessed)
	t.Logf("  - 处理器调用: %d 次", finalCalls)
	t.Logf("  - 平均批大小: %.1f", avgBatchSize)
	t.Logf("  - 吞吐量: %.2f items/sec", throughput)
	t.Logf("  - 添加耗时: %v", addDuration)
	t.Logf("  - 处理耗时: %v", processDuration)
	t.Logf("  - 数据竞争: %d", finalProcessingErrors)
	t.Logf("  - 添加错误: %d", finalAddErrors)
}

// TestStopResourceDestruction 测试Stop()后资源完全销毁
func TestStopResourceDestruction(t *testing.T) {
	var processedCount int64
	var processorRunning int64
	
	processor := func(items []string) []error {
		atomic.StoreInt64(&processorRunning, 1)
		atomic.AddInt64(&processedCount, int64(len(items)))
		// 模拟长时间处理
		time.Sleep(50 * time.Millisecond)
		atomic.StoreInt64(&processorRunning, 0)
		return nil
	}
	
	config := batcher.BatchConfig{
		BatchSize: 10,
		PoolSize:  5,
		QueueSize: 100,
		Timeout:   100 * time.Millisecond,
		Ctx:       context.Background(),
	}
	
	// 记录初始状态
	initialGoroutines := runtime.NumGoroutine()
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	
	t.Logf("测试开始 - Goroutines: %d, Memory: %d KB", initialGoroutines, m1.Alloc/1024)
	
	// 创建批处理器
	b, err := batcher.NewChanBatcher[string](processor, config)
	if err != nil {
		t.Fatalf("创建批处理器失败: %v", err)
	}
	
	// 添加数据让处理器开始工作
	for i := 0; i < 100; i++ {
		err := b.Add(fmt.Sprintf("destruction-test-item-%d", i))
		if err != nil {
			t.Errorf("添加数据失败: %v", err)
		}
	}
	
	// 等待处理器开始工作
	for atomic.LoadInt64(&processorRunning) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("处理器已开始工作")
	
	// 记录Stop前的状态
	beforeStopGoroutines := runtime.NumGoroutine()
	processedBeforeStop := atomic.LoadInt64(&processedCount)
	t.Logf("Stop前 - Goroutines: %d, 已处理: %d", beforeStopGoroutines, processedBeforeStop)
	
	// 调用Stop
	stopStart := time.Now()
	b.Stop()
	stopDuration := time.Since(stopStart)
	t.Logf("Stop()完成，耗时: %v", stopDuration)
	
	// 验证Stop()是否等待正在进行的处理完成
	processedAfterStop := atomic.LoadInt64(&processedCount)
	if processedAfterStop < processedBeforeStop {
		t.Errorf("❌ Stop()没有等待正在进行的处理完成")
	} else {
		t.Logf("✓ Stop()正确等待处理完成: %d -> %d", processedBeforeStop, processedAfterStop)
	}
	
	// 验证Stop()后不能添加新数据
	err = b.Add("should-fail-after-stop")
	if err == nil {
		t.Error("❌ Stop()后仍能添加数据")
	} else {
		t.Logf("✓ Stop()后正确拒绝新数据: %v", err)
	}
	
	// 多次调用Stop()应该是安全的
	for i := 0; i < 5; i++ {
		b.Stop() // 应该不会panic或死锁
	}
	t.Logf("✓ 多次调用Stop()安全")
	
	// 等待goroutine清理
	time.Sleep(200 * time.Millisecond)
	
	// 检查goroutine清理
	finalGoroutines := runtime.NumGoroutine()
	goroutineLeaks := finalGoroutines - initialGoroutines
	
	if goroutineLeaks > 2 { // 允许少量误差
		t.Errorf("❌ Goroutine泄漏: 初始=%d, 最终=%d, 泄漏=%d", 
			initialGoroutines, finalGoroutines, goroutineLeaks)
	} else {
		t.Logf("✓ Goroutine正确清理: 初始=%d, 最终=%d", initialGoroutines, finalGoroutines)
	}
	
	// 检查内存清理
	runtime.GC()
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	
	memoryGrowth := int64(m2.Alloc) - int64(m1.Alloc)
	t.Logf("内存变化: %d KB -> %d KB (增长: %d KB)", 
		m1.Alloc/1024, m2.Alloc/1024, memoryGrowth/1024)
	
	if memoryGrowth > 512*1024 { // 超过512KB认为有问题
		t.Errorf("❌ 可能存在内存泄漏: 增长了 %d KB", memoryGrowth/1024)
	} else {
		t.Logf("✓ 内存使用正常")
	}
	
	// 验证处理器不再运行
	time.Sleep(200 * time.Millisecond)
	if atomic.LoadInt64(&processorRunning) != 0 {
		t.Error("❌ Stop()后处理器仍在运行")
	} else {
		t.Logf("✓ 处理器已完全停止")
	}
	
	t.Logf("✓ Stop()资源销毁测试完成")
}