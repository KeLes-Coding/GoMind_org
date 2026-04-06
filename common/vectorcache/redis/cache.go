package rediscache

import (
	"GopherAI/common/observability"
	commonredis "GopherAI/common/redis"
	"GopherAI/common/vectorcache"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	redisCli "github.com/redis/go-redis/v9"
)

const (
	queryKeyPrefix        = "rag:query"
	fileQuerySetKeyPrefix = "rag:file:queries"
	indexedKeyPrefix      = "rag:file-indexed"
	defaultScanBatch      = 100
)

// Cache 是基于 Redis 的 VectorCache 实现。
// 这层的所有失败都应该允许主链路回落到 Milvus，因此这里的方法只返回错误，不做 panic。
type Cache struct{}

// NewCache 返回 Redis 检索缓存实现。
func NewCache() *Cache {
	return &Cache{}
}

// GetQueryDocuments 读取查询结果缓存。
func (c *Cache) GetQueryDocuments(ctx context.Context, key vectorcache.QueryKey) ([]vectorcache.CachedDocument, bool, error) {
	if !commonredis.IsAvailable() || commonredis.Rdb == nil {
		observability.RecordRAGCacheLookup(false)
		return nil, false, nil
	}

	payload, err := commonredis.Rdb.Get(ctx, buildQueryCacheKey(key)).Result()
	if err != nil {
		if err == redisCli.Nil {
			observability.RecordRAGCacheLookup(false)
			return nil, false, nil
		}
		observability.RecordRAGCacheLookup(false)
		return nil, false, err
	}

	var docs []vectorcache.CachedDocument
	if err := json.Unmarshal([]byte(payload), &docs); err != nil {
		observability.RecordRAGCacheLookup(false)
		return nil, false, err
	}
	observability.RecordRAGCacheLookup(true)
	return docs, true, nil
}

// SetQueryDocuments 写入查询结果缓存，并记录 file_id -> query_key 的反向关联。
// 这样删除文件或重建索引时，就可以按 file_id 做定向缓存失效。
func (c *Cache) SetQueryDocuments(ctx context.Context, key vectorcache.QueryKey, docs []vectorcache.CachedDocument, ttl time.Duration) error {
	if !commonredis.IsAvailable() || commonredis.Rdb == nil {
		return nil
	}
	if ttl <= 0 {
		return nil
	}

	cacheKey := buildQueryCacheKey(key)
	payload, err := json.Marshal(docs)
	if err != nil {
		return err
	}
	pipe := commonredis.Rdb.TxPipeline()
	pipe.Set(ctx, cacheKey, payload, ttl)

	fileIDs := collectFileIDs(docs)
	for _, fileID := range fileIDs {
		if strings.TrimSpace(fileID) == "" {
			continue
		}
		setKey := buildFileQuerySetKey(fileID)
		pipe.SAdd(ctx, setKey, cacheKey)
		pipe.Expire(ctx, setKey, ttl)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// GetIndexedFileVersion 读取 file_id + version 维度的正式入库存在性缓存。
func (c *Cache) GetIndexedFileVersion(ctx context.Context, fileID string, version int) (bool, error) {
	if !commonredis.IsAvailable() || commonredis.Rdb == nil {
		return false, nil
	}

	value, err := commonredis.Rdb.Get(ctx, buildIndexedFileVersionKey(fileID, version)).Result()
	if err != nil {
		if err == redisCli.Nil {
			return false, nil
		}
		return false, err
	}
	return value == "1", nil
}

// SetIndexedFileVersion 只缓存正向存在性结果。
// miss 仍然会回落到 Milvus 查询，避免把短期未入库和永久不存在混在一起。
func (c *Cache) SetIndexedFileVersion(ctx context.Context, fileID string, version int, ttl time.Duration) error {
	if !commonredis.IsAvailable() || commonredis.Rdb == nil {
		return nil
	}
	if strings.TrimSpace(fileID) == "" || version <= 0 || ttl <= 0 {
		return nil
	}
	return commonredis.Rdb.Set(ctx, buildIndexedFileVersionKey(fileID, version), "1", ttl).Err()
}

// InvalidateFile 按 file_id 失效查询缓存和存在性缓存。
// 第一阶段优先保证正确性，因此宁可多删一点，也不能让旧 chunk 长时间留在缓存里。
func (c *Cache) InvalidateFile(ctx context.Context, fileID string) error {
	if !commonredis.IsAvailable() || commonredis.Rdb == nil {
		return nil
	}
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil
	}

	var keys []string
	setKey := buildFileQuerySetKey(fileID)
	queryKeys, err := commonredis.Rdb.SMembers(ctx, setKey).Result()
	if err != nil && err != redisCli.Nil {
		return err
	}
	keys = append(keys, queryKeys...)
	keys = append(keys, setKey)

	pattern := buildIndexedFileVersionPattern(fileID)
	var cursor uint64
	for {
		matched, nextCursor, err := commonredis.Rdb.Scan(ctx, cursor, pattern, defaultScanBatch).Result()
		if err != nil {
			return err
		}
		keys = append(keys, matched...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if len(keys) == 0 {
		return nil
	}
	deletedKeys := deduplicateKeys(keys)
	if err := commonredis.Rdb.Del(ctx, deletedKeys...).Err(); err != nil {
		return err
	}
	observability.RecordRAGCacheInvalidation(len(deletedKeys))
	return nil
}

// InvalidateScope 按 owner/status/kb 范围粗粒度删除查询缓存。
// 这一步用于知识库重建或批量状态变更场景，优先保证正确性。
func (c *Cache) InvalidateScope(ctx context.Context, scope vectorcache.InvalidationScope) error {
	if !commonredis.IsAvailable() || commonredis.Rdb == nil {
		return nil
	}
	if scope.OwnerID <= 0 {
		return nil
	}

	pattern := buildScopePattern(scope)

	var (
		cursor uint64
		keys   []string
	)
	for {
		matched, nextCursor, err := commonredis.Rdb.Scan(ctx, cursor, pattern, defaultScanBatch).Result()
		if err != nil {
			return err
		}
		keys = append(keys, matched...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	keys = deduplicateKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	if err := commonredis.Rdb.Del(ctx, keys...).Err(); err != nil {
		return err
	}
	observability.RecordRAGCacheInvalidation(len(keys))
	return nil
}

func buildScopePattern(scope vectorcache.InvalidationScope) string {
	return fmt.Sprintf("%s:%d:%s:%s:*",
		queryKeyPrefix,
		scope.OwnerID,
		normalizeEmptyTagValue(scope.Status),
		normalizeEmptyTagValue(scope.KBID),
	)
}

func buildQueryCacheKey(key vectorcache.QueryKey) string {
	normalizedQuery := normalizeQuery(key.Query)
	if key.TopK <= 0 {
		key.TopK = 5
	}
	return fmt.Sprintf("%s:%d:%s:%s:%d:%s",
		queryKeyPrefix,
		key.OwnerID,
		normalizeEmptyTagValue(key.Status),
		normalizeEmptyTagValue(key.KBID),
		key.TopK,
		hashString(normalizedQuery),
	)
}

func buildFileQuerySetKey(fileID string) string {
	return fmt.Sprintf("%s:%s", fileQuerySetKeyPrefix, strings.TrimSpace(fileID))
}

func buildIndexedFileVersionKey(fileID string, version int) string {
	return fmt.Sprintf("%s:%s:%d", indexedKeyPrefix, strings.TrimSpace(fileID), version)
}

func buildIndexedFileVersionPattern(fileID string) string {
	return fmt.Sprintf("%s:%s:*", indexedKeyPrefix, strings.TrimSpace(fileID))
}

func normalizeQuery(query string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(strings.ToLower(query))), " ")
}

func normalizeEmptyTagValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "__empty__"
	}
	return value
}

func hashString(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func collectFileIDs(docs []vectorcache.CachedDocument) []string {
	result := make([]string, 0, len(docs))
	seen := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		fileID := metadataString(doc.MetaData, "file_id")
		if fileID == "" {
			continue
		}
		if _, ok := seen[fileID]; ok {
			continue
		}
		seen[fileID] = struct{}{}
		result = append(result, fileID)
	}
	return result
}

func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return ""
	}

	switch value := raw.(type) {
	case string:
		return value
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	}
	return fmt.Sprintf("%v", raw)
}

func deduplicateKeys(keys []string) []string {
	result := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}
