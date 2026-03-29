package redis

import (
	"GopherAI/config"
	"fmt"
)

const unifiedRAGIndexScope = "__shared__"

// key:特定邮箱-> 验证码
func GenerateCaptcha(email string) string {
	return fmt.Sprintf(config.DefaultRedisKeyConfig.CaptchaPrefix, email)
}

func GenerateIndexName(filename string) string {
	indexName := fmt.Sprintf(config.DefaultRedisKeyConfig.IndexName, filename)
	return indexName
}

func GenerateIndexNamePrefix(filename string) string {
	prefix := fmt.Sprintf(config.DefaultRedisKeyConfig.IndexNamePrefix, filename)
	return prefix
}

// GenerateUnifiedRAGIndexName 返回统一检索入口使用的共享 RediSearch 索引名。
// 它和按文件拆分的旧索引并存存在，便于新链路渐进迁移、旧链路继续兜底。
func GenerateUnifiedRAGIndexName() string {
	return GenerateIndexName(unifiedRAGIndexScope)
}

// GenerateUnifiedRAGIndexPrefix 返回共享索引对应的 hash key 前缀。
// 新增的 chunk 文档会落到这套独立前缀下，避免把历史按文件索引遗留的 hash
// 直接暴露给统一检索入口，从而引入删除脏数据回流问题。
func GenerateUnifiedRAGIndexPrefix() string {
	return GenerateIndexNamePrefix(unifiedRAGIndexScope)
}

// GenerateSessionLockKey 为 session 维度的分布式锁生成 Redis key。
func GenerateSessionLockKey(sessionID string) string {
	return fmt.Sprintf("ai:session:lock:%s", sessionID)
}

// GenerateSessionHotStateKey 为 session 热状态快照生成 Redis key。
func GenerateSessionHotStateKey(sessionID string) string {
	return fmt.Sprintf("ai:session:hot:%s", sessionID)
}
