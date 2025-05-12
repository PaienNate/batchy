package awesomeProject21

import (
	"context"
	"errors"
	"github.com/panjf2000/ants/v2"
	"time"
)

// ChanBatcherInstance 阻塞式批处理器（有缓冲channel）
type ChanBatcherInstance[T any] struct {
	processor Processor[T]
	itemLimit int
	queue     chan T // 有缓冲channel
	workers   *ants.Pool
	ctx       context.Context
	cancel    context.CancelFunc
	timeout   time.Duration
}

// NewChanBatcher 创建阻塞式批处理器
func NewChanBatcher[T any](
	processor Processor[T],
	batchConfig BatchConfig,
) (*ChanBatcherInstance[T], error) {
	var err error
	if batchConfig.Ctx == nil {
		batchConfig.Ctx = context.Background()
	}
	if batchConfig.QueueSize == 0 {
		batchConfig.QueueSize = 1000 * batchConfig.BatchSize
	}
	if batchConfig.PoolSize <= 0 {
		return nil, ErrWorkerNotSet
	}
	if batchConfig.Timeout == 0 {
		return nil, ErrInvalidTimeout
	}
	if processor == nil {
		return nil, ErrProcessorNotSet
	}

	ctx, cancel := context.WithCancel(batchConfig.Ctx)

	// 有缓冲channel（缓冲区大小=10倍批处理量）
	instance := &ChanBatcherInstance[T]{
		processor: processor,
		itemLimit: batchConfig.BatchSize,
		queue:     make(chan T, batchConfig.QueueSize), // 关键点：有缓冲但会阻塞
		ctx:       ctx,
		cancel:    cancel,
		timeout:   batchConfig.Timeout,
	}

	// 创建工作池
	pool, err := ants.NewPool(batchConfig.PoolSize)
	if err != nil {
		return nil, err
	}
	instance.workers = pool

	// 启动worker
	for i := 0; i < batchConfig.PoolSize; i++ {
		pool.Submit(func() { instance.worker() })
	}

	return instance, nil
}

// Add 方法（完全阻塞式）
func (c *ChanBatcherInstance[T]) Add(item T) error {
	select {
	case <-c.ctx.Done():
		return errors.New("batcher已停止")
	case c.queue <- item: // 关键点：channel满时会自动阻塞
		return nil
	}
}

// worker 核心逻辑
func (c *ChanBatcherInstance[T]) worker() {
	buffer := make([]T, 0, c.itemLimit)
	timer := time.NewTimer(c.timeout)
	defer timer.Stop()

	for {
		select {
		case <-c.ctx.Done():
			// 处理剩余数据
			if len(buffer) > 0 {
				c.processor(buffer)
			}
			return

		case item := <-c.queue:
			buffer = append(buffer, item)
			if len(buffer) >= c.itemLimit {
				c.processor(buffer)
				buffer = buffer[:0]
				timer.Reset(c.timeout)
			}

		case <-timer.C:
			// 超时处理
			if len(buffer) > 0 {
				c.processor(buffer)
				buffer = buffer[:0]
			}
			timer.Reset(c.timeout)
		}
	}
}

// Stop 停止批处理器
func (c *ChanBatcherInstance[T]) Stop() {
	c.cancel()          // 通知所有worker退出
	c.workers.Release() // 释放协程池
	close(c.queue)      // 关闭channel（可选）
}
