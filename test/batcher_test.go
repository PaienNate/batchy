package test

import (
	"context"
	"fmt"
	batcher "github.com/PaienNate/batchy"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatcherPerformance(t *testing.T) {
	const (
		totalProducers   = 3000                              // 生产者数量
		itemsPerProducer = 1000                              // 每个生产者生产的数据量
		totalItems       = totalProducers * itemsPerProducer // 总数据量
		batchSize        = 1000                              // 批处理大小
		consumerPoolSize = 30                                // 消费者数量
		timeout          = 1000 * time.Millisecond           // 批处理超时时间
	)

	// 初始化数据库
	db := initDB()

	// 使用原子计数器跟踪已处理的项目数
	var processedCount int64

	// 实现处理函数 - 批量插入数据库
	processor := func(items []ComplexMetrics) []error {
		// 批量插入数据
		_ = db.CreateInBatches(items, batchSize)

		// 更新已处理计数
		atomic.AddInt64(&processedCount, int64(len(items)))

		return nil
	}
	config := batcher.BatchConfig{
		BatchSize: batchSize,
		PoolSize:  consumerPoolSize,
		QueueSize: max(5000, batchSize*consumerPoolSize*2),
		Ctx:       context.Background(),
		Timeout:   timeout,
	}
	// 创建批处理器
	ringBatcher, err := batcher.NewRingBatcher[ComplexMetrics](processor, config)
	if err != nil {
		t.Fatalf("Failed to create ringBatcher: %v", err)
	}

	// ringBatcher := NewChanBatcher[ComplexMetrics](batchSize, consumerPoolSize, processor, timeout)

	// 开始计时
	startTime := time.Now()

	// 启动生产者协程
	var wg sync.WaitGroup
	wg.Add(totalProducers)

	for p := 0; p < totalProducers; p++ {
		go func(producerID int) {
			defer wg.Done()

			for i := 0; i < itemsPerProducer; i++ {
				// 生成随机复杂指标数据
				item := ComplexMetrics{
					Timestamp:   time.Now(),
					DeviceID:    fmt.Sprintf("device-%d-%d", producerID, i),
					Temperature: rand.Float64()*50 + 10,   // 10-60°C
					Humidity:    rand.Float64() * 100,     // 0-100%
					Pressure:    rand.Float64()*200 + 800, // 800-1000hPa
					Status:      Status(rand.Intn(3)),
					Metadata: JSONB{
						"location":   fmt.Sprintf("area-%d", rand.Intn(10)),
						"version":    fmt.Sprintf("v%d.%d", rand.Intn(5), rand.Intn(10)),
						"extra_data": rand.Float64(),
					},
				}
				err := ringBatcher.Add(item)
				if err != nil {
					fmt.Println("Error adding item:", err)
				}
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			}
		}(p)
	}

	// 等待所有生产者完成
	wg.Wait()

	// 等待处理完所有项目
	for atomic.LoadInt64(&processedCount) < totalItems {
		time.Sleep(100 * time.Millisecond)
	}

	// 停止批处理器
	ringBatcher.Stop()

	// 计算总时间
	duration := time.Since(startTime)

	// 验证结果
	if atomic.LoadInt64(&processedCount) != totalItems {
		t.Errorf("处理的项目数量不匹配: 预期 %d, 实际 %d", totalItems, processedCount)
	}

	// 检查数据库中的记录数
	var dbCount int64
	db.Model(&ComplexMetrics{}).Count(&dbCount)
	if dbCount != totalItems {
		t.Errorf("数据库中的记录数不匹配: 预期 %d, 实际 %d", totalItems, dbCount)
	}

	// 输出性能数据
	t.Logf("总项目数: %d", totalItems)
	t.Logf("处理时间: %v", duration)
	t.Logf("每秒处理项目数: %.2f", float64(totalItems)/duration.Seconds())
	t.Logf("数据库记录数: %d", dbCount)

	fmt.Printf("测试完成，总数据量: %d, 总时间: %v, 每秒处理量: %.2f, 数据库记录数: %d\n",
		totalItems, duration, float64(totalItems)/duration.Seconds(), dbCount)
}
