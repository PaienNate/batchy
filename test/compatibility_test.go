package test

import (
	"context"
	"testing"
	"time"

	batcher "github.com/PaienNate/batchy"
)

// TestBackwardCompatibility 测试向后兼容性
func TestBackwardCompatibility(t *testing.T) {
	// 测试原有的初始化方式是否仍然有效
	processor := func(items []string) []error {
		t.Logf("Processing batch of %d items", len(items))
		return nil
	}

	// 测试1: 基本配置兼容性
	config1 := batcher.BatchConfig{
		BatchSize: 10,
		PoolSize:  2,
		Timeout:   100 * time.Millisecond,
	}

	b1, err := batcher.NewChanBatcher[string](processor, config1)
	if err != nil {
		t.Fatalf("Basic config compatibility failed: %v", err)
	}

	// 测试添加数据
	err = b1.Add("test1")
	if err != nil {
		t.Errorf("Add method compatibility failed: %v", err)
	}

	b1.Stop()
	t.Log("Basic compatibility test passed")

	// 测试2: 带Context的配置兼容性
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config2 := batcher.BatchConfig{
		BatchSize: 5,
		PoolSize:  1,
		Timeout:   50 * time.Millisecond,
		Ctx:       ctx,
	}

	b2, err := batcher.NewChanBatcher[string](processor, config2)
	if err != nil {
		t.Fatalf("Context config compatibility failed: %v", err)
	}

	b2.Stop()
	t.Log("Context compatibility test passed")

	// 测试3: 新功能的默认值兼容性
	config3 := batcher.BatchConfig{
		BatchSize: 20,
		PoolSize:  3,
		Timeout:   200 * time.Millisecond,
		// 不设置新的字段，应该使用默认值
	}

	b3, err := batcher.NewChanBatcher[string](processor, config3)
	if err != nil {
		t.Fatalf("Default values compatibility failed: %v", err)
	}

	b3.Stop()
	t.Log("Default values compatibility test passed")

	// 测试4: 验证新功能可选启用
	config4 := batcher.BatchConfig{
		BatchSize:         15,
		PoolSize:          2,
		Timeout:           150 * time.Millisecond,
		DynamicBatching:   true,
		SchedulingPolicy:  batcher.ROUND_ROBIN,
		MinBatchSize:      5,
		MaxBatchSize:      30,
		AdaptiveThreshold: 75 * time.Millisecond,
	}

	b4, err := batcher.NewChanBatcher[string](processor, config4)
	if err != nil {
		t.Fatalf("New features compatibility failed: %v", err)
	}

	b4.Stop()
	t.Log("New features compatibility test passed")

	t.Log("All backward compatibility tests passed successfully")
}

// TestLegacyInterface 测试遗留接口兼容性
func TestLegacyInterface(t *testing.T) {
	processor := func(items []int) []error {
		return nil
	}

	config := batcher.BatchConfig{
		BatchSize: 5,
		PoolSize:  1,
		Timeout:   100 * time.Millisecond,
	}

	// 测试接口方法是否保持一致
	var b batcher.Batcher[int]
	b, err := batcher.NewChanBatcher[int](processor, config)
	if err != nil {
		t.Fatalf("Interface compatibility failed: %v", err)
	}

	// 测试Add方法
	err = b.Add(42)
	if err != nil {
		t.Errorf("Add method interface compatibility failed: %v", err)
	}

	// 测试Stop方法
	b.Stop()

	t.Log("Legacy interface compatibility test passed")
}