package aihelper

import (
	"context"
	"fmt"
	"sync"
)

// modelConcurrencyLimits 定义不同模型类型默认允许的并发数。
// 这里的目标不是精确限流，而是先给最重的下游模型调用增加一道“实例内并发闸门”，
// 防止某种模型被瞬时高并发打满后，拖垮整台服务。
var modelConcurrencyLimits = map[string]int{
	ModelTypeOpenAI: 6,
	ModelTypeRAG:    4,
	ModelTypeMCP:    3,
	ModelTypeOllama: 2,
}

// modelConcurrencyManager 用带缓冲 channel 的方式实现轻量 semaphore。
// 之所以放在 aihelper 层，是因为这里最接近真实模型调用点，
// 可以同时覆盖同步回复、流式回复和摘要生成三类场景。
type modelConcurrencyManager struct {
	mu       sync.Mutex
	permits  map[string]chan struct{}
	capacity map[string]int
}

func newModelConcurrencyManager() *modelConcurrencyManager {
	return &modelConcurrencyManager{
		permits:  make(map[string]chan struct{}),
		capacity: make(map[string]int),
	}
}

func (m *modelConcurrencyManager) getPermitChan(modelType string) chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ch, exists := m.permits[modelType]; exists {
		return ch
	}

	capacity := modelConcurrencyLimits[modelType]
	if capacity <= 0 {
		capacity = 2
	}

	ch := make(chan struct{}, capacity)
	m.permits[modelType] = ch
	m.capacity[modelType] = capacity
	return ch
}

func (m *modelConcurrencyManager) acquire(ctx context.Context, modelType string) (func(), error) {
	permitChan := m.getPermitChan(modelType)

	select {
	case permitChan <- struct{}{}:
		return func() {
			<-permitChan
		}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("acquire model concurrency permit failed: %w", ctx.Err())
	}
}

var globalModelConcurrencyManager = newModelConcurrencyManager()

