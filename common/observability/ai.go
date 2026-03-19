package observability

import (
	"sync"
	"time"
)

// AISnapshot 是 AI 模块当前观测数据的只读快照。
type AISnapshot struct {
	RequestStats       []RequestStat `json:"request_stats"`
	ModelStats         []ModelStat   `json:"model_stats"`
	RequestTotal       int64         `json:"request_total"`
	RequestSuccess     int64         `json:"request_success"`
	RequestFailed      int64         `json:"request_failed"`
	ModelCallTotal     int64         `json:"model_call_total"`
	SummaryRefresh     int64         `json:"summary_refresh"`
	SummaryRefreshFail int64         `json:"summary_refresh_fail"`
	SummaryUsedTotal   int64         `json:"summary_used_total"`
	StreamActive       int64         `json:"stream_active"`
	StreamDisconnect   int64         `json:"stream_disconnect"`
	MQPublishSuccess   int64         `json:"mq_publish_success"`
	MQPublishFail      int64         `json:"mq_publish_fail"`
	MQConsumeSuccess   int64         `json:"mq_consume_success"`
	MQConsumeFail      int64         `json:"mq_consume_fail"`
	MQNackTotal        int64         `json:"mq_nack_total"`
	MQAckFailTotal     int64         `json:"mq_ack_fail_total"`
}

// RequestStat 是按“操作 + 模型类型”聚合后的请求统计。
type RequestStat struct {
	Operation      string `json:"operation"`
	ModelType      string `json:"model_type"`
	Total          int64  `json:"total"`
	Success        int64  `json:"success"`
	Failed         int64  `json:"failed"`
	LatencyMsTotal int64  `json:"latency_ms_total"`
	LatencyMsMax   int64  `json:"latency_ms_max"`
}

// ModelStat 是按“模型调用类型 + 模型类型”聚合后的调用统计。
type ModelStat struct {
	Operation        string `json:"operation"`
	ModelType        string `json:"model_type"`
	CallTotal        int64  `json:"call_total"`
	Success          int64  `json:"success"`
	Failed           int64  `json:"failed"`
	LatencyMsTotal   int64  `json:"latency_ms_total"`
	LatencyMsMax     int64  `json:"latency_ms_max"`
	ContextMessages  int64  `json:"context_messages_total"`
	SummaryUsedTotal int64  `json:"summary_used_total"`
}

type requestCounter struct {
	Operation      string
	ModelType      string
	Total          int64
	Success        int64
	Failed         int64
	LatencyMsTotal int64
	LatencyMsMax   int64
}

type modelCounter struct {
	Operation        string
	ModelType        string
	CallTotal        int64
	Success          int64
	Failed           int64
	LatencyMsTotal   int64
	LatencyMsMax     int64
	ContextMessages  int64
	SummaryUsedTotal int64
}

type aiObserver struct {
	mu sync.Mutex

	requests map[string]*requestCounter
	models   map[string]*modelCounter

	requestTotal       int64
	requestSuccess     int64
	requestFailed      int64
	modelCallTotal     int64
	summaryRefresh     int64
	summaryRefreshFail int64
	summaryUsedTotal   int64
	streamActive       int64
	streamDisconnect   int64
	mqPublishSuccess   int64
	mqPublishFail      int64
	mqConsumeSuccess   int64
	mqConsumeFail      int64
	mqNackTotal        int64
	mqAckFailTotal     int64
}

var globalAIObserver = &aiObserver{
	requests: make(map[string]*requestCounter),
	models:   make(map[string]*modelCounter),
}

func requestKey(operation string, modelType string) string {
	return operation + "|" + modelType
}

func modelKey(operation string, modelType string) string {
	return operation + "|" + modelType
}

func maxInt64(current int64, next int64) int64 {
	if next > current {
		return next
	}
	return current
}

// RecordRequest 记录一次 AI 业务请求结果。
func RecordRequest(operation string, modelType string, success bool, duration time.Duration) {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	key := requestKey(operation, modelType)
	counter, ok := globalAIObserver.requests[key]
	if !ok {
		counter = &requestCounter{Operation: operation, ModelType: modelType}
		globalAIObserver.requests[key] = counter
	}

	latencyMs := duration.Milliseconds()
	counter.Total++
	counter.LatencyMsTotal += latencyMs
	counter.LatencyMsMax = maxInt64(counter.LatencyMsMax, latencyMs)

	globalAIObserver.requestTotal++
	if success {
		counter.Success++
		globalAIObserver.requestSuccess++
	} else {
		counter.Failed++
		globalAIObserver.requestFailed++
	}
}

// RecordModelCall 记录一次真正发向模型的调用。
func RecordModelCall(operation string, modelType string, success bool, duration time.Duration, contextMessages int, usedSummary bool) {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	key := modelKey(operation, modelType)
	counter, ok := globalAIObserver.models[key]
	if !ok {
		counter = &modelCounter{Operation: operation, ModelType: modelType}
		globalAIObserver.models[key] = counter
	}

	latencyMs := duration.Milliseconds()
	counter.CallTotal++
	counter.LatencyMsTotal += latencyMs
	counter.LatencyMsMax = maxInt64(counter.LatencyMsMax, latencyMs)
	counter.ContextMessages += int64(contextMessages)
	globalAIObserver.modelCallTotal++

	if usedSummary {
		counter.SummaryUsedTotal++
		globalAIObserver.summaryUsedTotal++
	}

	if success {
		counter.Success++
	} else {
		counter.Failed++
	}
}

// RecordSummaryRefresh 记录摘要刷新结果。
func RecordSummaryRefresh(success bool) {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	if success {
		globalAIObserver.summaryRefresh++
		return
	}
	globalAIObserver.summaryRefreshFail++
}

// RecordStreamActiveDelta 更新当前活跃流式请求数。
func RecordStreamActiveDelta(delta int64) {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	globalAIObserver.streamActive += delta
	if globalAIObserver.streamActive < 0 {
		globalAIObserver.streamActive = 0
	}
}

// RecordStreamDisconnect 记录一次客户端中断。
func RecordStreamDisconnect() {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	globalAIObserver.streamDisconnect++
}

// RecordMQPublish 记录 MQ 发布结果。
func RecordMQPublish(success bool) {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	if success {
		globalAIObserver.mqPublishSuccess++
		return
	}
	globalAIObserver.mqPublishFail++
}

// RecordMQConsume 记录 MQ 消费处理结果。
func RecordMQConsume(success bool) {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	if success {
		globalAIObserver.mqConsumeSuccess++
		return
	}
	globalAIObserver.mqConsumeFail++
}

// RecordMQNack 记录一次 nack / requeue。
func RecordMQNack() {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	globalAIObserver.mqNackTotal++
}

// RecordMQAckFail 记录一次 ack 失败。
func RecordMQAckFail() {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	globalAIObserver.mqAckFailTotal++
}

// SnapshotAI 返回当前 AI 可观测性的完整快照。
func SnapshotAI() AISnapshot {
	globalAIObserver.mu.Lock()
	defer globalAIObserver.mu.Unlock()

	requestStats := make([]RequestStat, 0, len(globalAIObserver.requests))
	for _, item := range globalAIObserver.requests {
		requestStats = append(requestStats, RequestStat{
			Operation:      item.Operation,
			ModelType:      item.ModelType,
			Total:          item.Total,
			Success:        item.Success,
			Failed:         item.Failed,
			LatencyMsTotal: item.LatencyMsTotal,
			LatencyMsMax:   item.LatencyMsMax,
		})
	}

	modelStats := make([]ModelStat, 0, len(globalAIObserver.models))
	for _, item := range globalAIObserver.models {
		modelStats = append(modelStats, ModelStat{
			Operation:        item.Operation,
			ModelType:        item.ModelType,
			CallTotal:        item.CallTotal,
			Success:          item.Success,
			Failed:           item.Failed,
			LatencyMsTotal:   item.LatencyMsTotal,
			LatencyMsMax:     item.LatencyMsMax,
			ContextMessages:  item.ContextMessages,
			SummaryUsedTotal: item.SummaryUsedTotal,
		})
	}

	return AISnapshot{
		RequestStats:       requestStats,
		ModelStats:         modelStats,
		RequestTotal:       globalAIObserver.requestTotal,
		RequestSuccess:     globalAIObserver.requestSuccess,
		RequestFailed:      globalAIObserver.requestFailed,
		ModelCallTotal:     globalAIObserver.modelCallTotal,
		SummaryRefresh:     globalAIObserver.summaryRefresh,
		SummaryRefreshFail: globalAIObserver.summaryRefreshFail,
		SummaryUsedTotal:   globalAIObserver.summaryUsedTotal,
		StreamActive:       globalAIObserver.streamActive,
		StreamDisconnect:   globalAIObserver.streamDisconnect,
		MQPublishSuccess:   globalAIObserver.mqPublishSuccess,
		MQPublishFail:      globalAIObserver.mqPublishFail,
		MQConsumeSuccess:   globalAIObserver.mqConsumeSuccess,
		MQConsumeFail:      globalAIObserver.mqConsumeFail,
		MQNackTotal:        globalAIObserver.mqNackTotal,
		MQAckFailTotal:     globalAIObserver.mqAckFailTotal,
	}
}
