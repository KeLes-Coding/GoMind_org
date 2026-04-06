package vectorstore

import "context"

// Document 描述一条待写入主向量存储的标准化 chunk 文档。
// 这一层故意不暴露具体数据库实现细节，只保留 RAG 主链路真正需要的数据。
type Document struct {
	ID       string
	Content  string
	Vector   []float32
	MetaData map[string]any
}

// SearchFilter 描述一次检索需要带上的业务过滤边界。
// 第一阶段先收口 owner/status/kb/file/storageKey 这些当前项目已稳定使用的字段。
type SearchFilter struct {
	OwnerID    int64
	Status     string
	KBID       string
	FileID     string
	StorageKey string
}

// SearchRequest 描述一次主向量检索请求。
type SearchRequest struct {
	Vector []float32
	TopK   int
	Filter SearchFilter
}

// SearchResult 描述向量库返回的一条标准化召回结果。
type SearchResult struct {
	ID       string
	Content  string
	Score    float32
	MetaData map[string]any
}

// Store 定义 RAG 主向量存储需要实现的最小接口集。
// 聊天层、worker 和文件管理层都只应依赖这组能力，而不直接感知 Milvus SDK。
type Store interface {
	EnsureCollection(ctx context.Context, dimension int) error
	UpsertDocuments(ctx context.Context, docs []Document) error
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
	DeleteByFileID(ctx context.Context, fileID string) error
	HasFileVersion(ctx context.Context, fileID string, version int) (bool, error)
}
