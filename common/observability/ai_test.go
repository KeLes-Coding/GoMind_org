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
