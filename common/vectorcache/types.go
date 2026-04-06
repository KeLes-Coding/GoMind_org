package vectorcache

import (
	"context"
	"time"
)

// QueryKey 描述一次检索缓存的业务边界。
// 这里故意不暴露底层 Redis key 细节，让上层只关心“这次查询是谁、在哪个范围、查了什么”。
type QueryKey struct {
	OwnerID int64
	Status  string
	KBID    string
	Query   string
	TopK    int
}

// InvalidationScope 描述一批查询缓存所属的业务范围。
// 当文件删除、知识库重建或状态批量切换发生时，可以按范围粗粒度失效缓存。
type InvalidationScope struct {
	OwnerID int64
	Status  string
	KBID    string
}

// CachedDocument 是缓存层保存的轻量检索结果。
// 它直接对齐 RAG 最终需要的字段，避免命中缓存后还要再做额外字段转换。
type CachedDocument struct {
	ID       string
	Content  string
	MetaData map[string]any
}

// Cache 定义 Redis 检索缓存层需要提供的最小能力。
// 主存储正确性仍然由 Milvus 保证，这里只负责命中加速和缓存失效。
type Cache interface {
	GetQueryDocuments(ctx context.Context, key QueryKey) ([]CachedDocument, bool, error)
	SetQueryDocuments(ctx context.Context, key QueryKey, docs []CachedDocument, ttl time.Duration) error
	GetIndexedFileVersion(ctx context.Context, fileID string, version int) (bool, error)
	SetIndexedFileVersion(ctx context.Context, fileID string, version int, ttl time.Duration) error
	InvalidateFile(ctx context.Context, fileID string) error
	InvalidateScope(ctx context.Context, scope InvalidationScope) error
}
