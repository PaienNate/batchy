package batchy

import (
	"context"
	"errors"
	"github.com/cenkalti/backoff/v4"
	"github.com/hedzr/go-ringbuf/v2"
	"github.com/hedzr/go-ringbuf/v2/mpmc"
	"github.com/panjf2000/ants/v2"
	"sync/atomic"
	"time"
)

var (
	ErrWorkerNotSet    = errors.New("worker pool size must be greater than zero")
	ErrProcessorNotSet = errors.New("processor function must not be nil")
	ErrInvalidTimeout  = errors.New("Timeout duration must be positive")
)

// TODO: 退避参数可配置

// Batcher [T any] can add an item of type T, returning the corresponding error
type Batcher[T any] interface {
	// Add adds an item to the current batch
	Add(T) error

	// Stop stops the BatcherInstance
	Stop()
}

type BatcherInstance[T any] struct {
	processor   Processor[T] // 处理函数，接受 []T 并返回 []error
	itemLimit   int          // 批处理的最大项目数
	stopped     atomic.Bool  // 是否已停止
	queue       mpmc.Queue[T]
	ants        *ants.Pool
	ctx         context.Context             // 控制是否要停止
	cancel      context.CancelFunc          // 停止函数
	timeout     time.Duration               // 多久之后会强制落盘
	baseBackoff *backoff.ExponentialBackOff // 预定义的配置模板
}

// Processor [T any] is a function that accepts items of type T and returns a corresponding array of errors
type Processor[T any] func(items []T) []error

type BatchConfig struct {
	// BatchSize 批大小
	BatchSize int
	// PoolSize 工作者数量
	PoolSize int
	// QueueSize 环形队列大小 默认 1000 * 批大小 这个存疑，多少合适我也不知道。
	QueueSize int
	// Ctx context 上下文
	Ctx context.Context
	// Timeout 若数据不足时，多久强制落盘一次
	Timeout time.Duration
}

// NewRingBatcher 新建一个Batch任务
// TODO：疑似有点BUG，会导致内存无限升高
func NewRingBatcher[T any](processor Processor[T], batchConfig BatchConfig) (Batcher[T], error) {
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
	ctx, cancel := context.WithCancel(batchConfig.Ctx) // claude改
	// 启动消费者并传入队列引用，每个的任务都是一样的：从queue里尝试取数据放在缓存，若数据 >= BatchSize，则执行processor（传入items）
	instance := &BatcherInstance[T]{
		processor: processor,
		itemLimit: batchConfig.BatchSize,
		queue:     ringbuf.New[T](uint32(batchConfig.QueueSize)), // ringbuf.New[T](uint32(batchConfig.QueueSize)),
		ctx:       ctx,                                           // claude改
		cancel:    cancel,                                        // claude改
		timeout:   batchConfig.Timeout,                           // claude改
	}
	instance.ants, err = ants.NewPool(batchConfig.PoolSize) // 协程数等于 BatchSize，根据实际负载调整
	if err != nil {
		return nil, err
	}
	// 预先启动这些消费者 TODO：或许可以改成一个先预热一部分，若发现Add阻塞，则启动更多的消费者，直到消费者数量达到池上限
	for i := 0; i < instance.ants.Cap(); i++ { // claude改: 使用for循环，因为Range返回的是索引和值
		err = instance.ants.Submit(instance.worker)
		if err != nil {
			cancel() // 确保取消context                //claude改
			return nil, err
		}
	}
	// 配置基础退避策略 TODO: 提供Config配置项
	base := backoff.NewExponentialBackOff()
	base.InitialInterval = 10 * time.Millisecond
	base.RandomizationFactor = 0.5
	base.MaxInterval = 300 * time.Millisecond
	base.MaxElapsedTime = 30 * time.Second
	instance.baseBackoff = base
	return instance, nil
}

func (b *BatcherInstance[T]) worker() {
	var buffer []T
	timer := time.NewTimer(b.timeout)
	defer timer.Stop()

	for {
		// 检查是否已停止                              //claude改
		if b.stopped.Load() { // claude改
			// 提交剩余数据                             //claude改
			if len(buffer) > 0 { // claude改
				b.processor(buffer) // claude改
			} // claude改
			return // claude改
		} // claude改

		select {
		case <-b.ctx.Done():
			// 提交剩余数据
			if len(buffer) > 0 {
				b.processor(buffer)
			}
			return

		case <-timer.C:
			// 超时触发提交
			if len(buffer) > 0 {
				b.processor(buffer)
				buffer = buffer[:0]
			}
			timer.Reset(b.timeout) // 重置定时器       //claude改: 无论buffer是否为空都重置定时器
		default:
			// 尝试从队列中取数据
			item, err := b.queue.Dequeue()
			if errors.Is(err, mpmc.ErrQueueEmpty) {
				// 检查context是否已取消                 //claude改
				select { // claude改
				case <-b.ctx.Done(): // claude改
					if len(buffer) > 0 { // claude改
						b.processor(buffer) // claude改
					} // claude改
					return // claude改
				default: // claude改
					time.Sleep(30 * time.Millisecond) // claude改
				} // claude改
				continue
			}

			buffer = append(buffer, item)

			// 数量触发提交
			if len(buffer) >= b.itemLimit {
				b.processor(buffer)
				buffer = buffer[:0]
				timer.Reset(b.timeout) // 重置定时器
			}
		}
	}
}

// Add 添加数据到队列中
func (b *BatcherInstance[T]) Add(item T) error {
	if b.stopped.Load() {
		return errors.New("batcher is stopped")
	}
	operation := func() error {
		return b.queue.Enqueue(item)
	}
	// 配置退避策略和时间限制
	// 复制基础配置模板（避免重复初始化）
	expBackoff := *b.baseBackoff // 值拷贝，生成新实例
	expBackoff.Reset()           // 重置状态（重要！）

	// 绑定上下文（支持外部主动取消，如 b.Ctx.Done()）
	ctxBackoff := backoff.WithContext(&expBackoff, b.ctx)
	// 执行重试（在30秒内无限重试，直到成功或超时）
	err := backoff.Retry(operation, ctxBackoff)
	if err != nil {
		return err
	}
	return nil
}

// Stop stops the BatcherInstance and prevents further additions
func (b *BatcherInstance[T]) Stop() {
	b.stopped.Store(true) // claude改
	b.ants.Release()      // claude改
	b.cancel()            // claude改
}
