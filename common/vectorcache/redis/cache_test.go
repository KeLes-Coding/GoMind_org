package rediscache

import (
	"GopherAI/common/vectorcache"
	"testing"
)

// TestBuildQueryCacheKey 验证查询缓存键会稳定归一化 query / status / kb / topK。
// 这样相同语义的重复问题才能命中同一份缓存，而不是因为大小写和空格差异被打散。
func TestBuildQueryCacheKey(t *testing.T) {
	keyA := buildQueryCacheKey(vectorcache.QueryKey{
		OwnerID: 7,
		Status:  "ready",
		KBID:    "kb-1",
		Query:   "  Hello   World  ",
		TopK:    5,
	})
	keyB := buildQueryCacheKey(vectorcache.QueryKey{
		OwnerID: 7,
		Status:  "ready",
		KBID:    "kb-1",
		Query:   "hello world",
		TopK:    5,
	})

	if keyA != keyB {
		t.Fatalf("expected normalized query cache keys to match, got %q and %q", keyA, keyB)
	}
}

// TestCollectFileIDs 验证缓存反向索引只会保留唯一 file_id。
// 这样单次查询命中同一文件多个 chunk 时，不会把同一个 query key 重复写入集合。
func TestCollectFileIDs(t *testing.T) {
	docs := []vectorcache.CachedDocument{
		{MetaData: map[string]any{"file_id": "file-1"}},
		{MetaData: map[string]any{"file_id": "file-1"}},
		{MetaData: map[string]any{"file_id": "file-2"}},
	}

	got := collectFileIDs(docs)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique file ids, got %d", len(got))
	}
	if got[0] != "file-1" || got[1] != "file-2" {
		t.Fatalf("unexpected file ids: %#v", got)
	}
}

// TestBuildScopePattern 验证范围级失效会生成稳定的 owner/status/kb 模式。
// 这样知识库重建时可以按范围清掉同一批查询缓存。
func TestBuildScopePattern(t *testing.T) {
	pattern := "rag:query:7:ready:kb-1:*"
	got := buildScopePattern(vectorcache.InvalidationScope{
		OwnerID: 7,
		Status:  "ready",
		KBID:    "kb-1",
	})
	if got != pattern {
		t.Fatalf("expected %q, got %q", pattern, got)
	}
}
