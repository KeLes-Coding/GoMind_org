package observability

import "testing"

// TestRecordRAGQuery 验证 RAG 检索观测计数能正确累计。
// 这能确保后续看 metrics 时，至少 hit / no-hit / chunk 数这些基础指标是可信的。
func TestRecordRAGQuery(t *testing.T) {
	globalAIObserver = &aiObserver{
		requests: make(map[string]*requestCounter),
		models:   make(map[string]*modelCounter),
	}

	RecordRAGQuery(true, 3, 2)
	RecordRAGQuery(false, 0, 0)

	snapshot := SnapshotAI()
	if snapshot.RAGQueryTotal != 2 {
		t.Fatalf("expected rag_query_total=2, got %d", snapshot.RAGQueryTotal)
	}
	if snapshot.RAGHitTotal != 1 {
		t.Fatalf("expected rag_hit_total=1, got %d", snapshot.RAGHitTotal)
	}
	if snapshot.RAGNoHitTotal != 1 {
		t.Fatalf("expected rag_no_hit_total=1, got %d", snapshot.RAGNoHitTotal)
	}
	if snapshot.RAGRetrievedChunksTotal != 3 {
		t.Fatalf("expected rag_retrieved_chunks_total=3, got %d", snapshot.RAGRetrievedChunksTotal)
	}
	if snapshot.RAGRetrievedFilesTotal != 2 {
		t.Fatalf("expected rag_retrieved_files_total=2, got %d", snapshot.RAGRetrievedFilesTotal)
	}
}

// TestRecordRAGFallback 验证 RAG 降级计数能单独累计。
// 这样后续排查“是没命中资料还是检索链路故障”时，至少有一条明确的统计线。
func TestRecordRAGFallback(t *testing.T) {
	globalAIObserver = &aiObserver{
		requests: make(map[string]*requestCounter),
		models:   make(map[string]*modelCounter),
	}

	RecordRAGFallback()
	RecordRAGFallback()

	snapshot := SnapshotAI()
	if snapshot.RAGFallbackTotal != 2 {
		t.Fatalf("expected rag_fallback_total=2, got %d", snapshot.RAGFallbackTotal)
	}
}

// TestRecordRAGCacheMetrics 验证 RAG 查询缓存命中、miss 和失效计数能被正确累计。
func TestRecordRAGCacheMetrics(t *testing.T) {
	globalAIObserver = &aiObserver{
		requests: make(map[string]*requestCounter),
		models:   make(map[string]*modelCounter),
	}

	RecordRAGCacheLookup(true)
	RecordRAGCacheLookup(false)
	RecordRAGCacheInvalidation(3)
	RecordRAGStoreMode("milvus_with_redis_cache")

	snapshot := SnapshotAI()
	if snapshot.RAGCacheHitTotal != 1 {
		t.Fatalf("expected rag_cache_hit_total=1, got %d", snapshot.RAGCacheHitTotal)
	}
	if snapshot.RAGCacheMissTotal != 1 {
		t.Fatalf("expected rag_cache_miss_total=1, got %d", snapshot.RAGCacheMissTotal)
	}
	if snapshot.RAGCacheInvalidationTotal != 3 {
		t.Fatalf("expected rag_cache_invalidation_total=3, got %d", snapshot.RAGCacheInvalidationTotal)
	}
	if snapshot.RAGStoreMode != "milvus_with_redis_cache" {
		t.Fatalf("expected rag_store_mode to be milvus_with_redis_cache, got %q", snapshot.RAGStoreMode)
	}
}

// TestRecordStreamSyncFailMetrics 验证第三阶段新增的 Redis 同步提交失败指标能够正确累计。
func TestRecordStreamSyncFailMetrics(t *testing.T) {
	globalAIObserver = &aiObserver{
		requests: make(map[string]*requestCounter),
		models:   make(map[string]*modelCounter),
	}

	RecordStreamMetaSyncFail()
	RecordStreamSnapshotSyncFail()
	RecordStreamChunkSyncFail()
	RecordStreamChunkSyncFail()

	snapshot := SnapshotAI()
	if snapshot.StreamMetaSyncFail != 1 {
		t.Fatalf("expected stream_meta_sync_fail=1, got %d", snapshot.StreamMetaSyncFail)
	}
	if snapshot.StreamSnapshotSyncFail != 1 {
		t.Fatalf("expected stream_snapshot_sync_fail=1, got %d", snapshot.StreamSnapshotSyncFail)
	}
	if snapshot.StreamChunkSyncFail != 2 {
		t.Fatalf("expected stream_chunk_sync_fail=2, got %d", snapshot.StreamChunkSyncFail)
	}
}

// TestRecordHelperExecutionCacheMetrics 验证 execution cache 的复用和释放指标能被正确累计。
func TestRecordHelperExecutionCacheMetrics(t *testing.T) {
	globalAIObserver = &aiObserver{
		requests: make(map[string]*requestCounter),
		models:   make(map[string]*modelCounter),
	}

	RecordHelperExecutionReuse()
	RecordHelperExecutionReuse()
	RecordHelperExecutionRelease()

	snapshot := SnapshotAI()
	if snapshot.HelperExecutionReuse != 2 {
		t.Fatalf("expected helper_execution_reuse_total=2, got %d", snapshot.HelperExecutionReuse)
	}
	if snapshot.HelperExecutionRelease != 1 {
		t.Fatalf("expected helper_execution_release_total=1, got %d", snapshot.HelperExecutionRelease)
	}
}

// TestRecordStage7DegradeMetrics 验证第七阶段新增的 DB 持久化失败、通知失败和 Redis 恢复降级指标。
func TestRecordStage7DegradeMetrics(t *testing.T) {
	globalAIObserver = &aiObserver{
		requests: make(map[string]*requestCounter),
		models:   make(map[string]*modelCounter),
	}

	RecordDBPersistFail()
	RecordDBPersistFail()
	RecordNotificationPublishFail()
	RecordStreamResumeRedisDegraded()

	snapshot := SnapshotAI()
	if snapshot.DBPersistFail != 2 {
		t.Fatalf("expected db_persist_fail=2, got %d", snapshot.DBPersistFail)
	}
	if snapshot.NotificationPublishFail != 1 {
		t.Fatalf("expected notification_publish_fail=1, got %d", snapshot.NotificationPublishFail)
	}
	if snapshot.StreamResumeRedisDegraded != 1 {
		t.Fatalf("expected stream_resume_redis_degraded_total=1, got %d", snapshot.StreamResumeRedisDegraded)
	}
}
