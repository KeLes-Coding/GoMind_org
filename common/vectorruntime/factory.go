package vectorruntime

import (
	"GopherAI/common/vectorstore"
	milvusstore "GopherAI/common/vectorstore/milvus"
	"GopherAI/config"
	"context"
	"strings"
)

const (
	// StoreModeMilvusPrimary 表示只启用 Milvus 主存储，不启用 Redis 检索缓存。
	StoreModeMilvusPrimary = "milvus_primary"
	// StoreModeMilvusWithRedisCache 表示 Milvus 主存储配合 Redis 检索缓存。
	StoreModeMilvusWithRedisCache = "milvus_with_redis_cache"
)

// CurrentStoreMode 返回当前已归一化的 RAG 存储模式。
// 当前代码库已经没有 Redis 主检索实现，因此灰度模式只保留两个可运行模式。
func CurrentStoreMode() string {
	mode := strings.TrimSpace(strings.ToLower(config.GetConfig().RagModelConfig.StoreMode))
	switch mode {
	case StoreModeMilvusPrimary:
		return StoreModeMilvusPrimary
	case StoreModeMilvusWithRedisCache:
		return StoreModeMilvusWithRedisCache
	default:
		return StoreModeMilvusWithRedisCache
	}
}

// IsCacheEnabled 判断当前模式是否启用 Redis 检索缓存。
func IsCacheEnabled() bool {
	return CurrentStoreMode() == StoreModeMilvusWithRedisCache
}

// NewConfiguredStore 根据当前配置返回主向量存储实现。
// 第一阶段收口后，主存储统一是 Milvus，只是是否叠加缓存层由模式决定。
func NewConfiguredStore(ctx context.Context, dimension int) (vectorstore.Store, error) {
	return milvusstore.NewStore(ctx, dimension)
}
