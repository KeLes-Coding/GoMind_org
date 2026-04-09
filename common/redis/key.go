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

// GenerateSessionOwnerLeaseKey 为 session owner lease 生成 Redis key。
func GenerateSessionOwnerLeaseKey(sessionID string) string {
	return fmt.Sprintf("ai:session:owner:%s", sessionID)
}

// GenerateSessionOwnerFenceKey 为 session owner fencing counter 生成 Redis key。
func GenerateSessionOwnerFenceKey(sessionID string) string {
	return fmt.Sprintf("ai:session:owner:fence:%s", sessionID)
}

// GenerateChatInstanceHeartbeatKey 为聊天实例心跳生成 Redis key。
func GenerateChatInstanceHeartbeatKey(instanceID string) string {
	return fmt.Sprintf("ai:instance:chat:%s", instanceID)
}

// GenerateSessionActiveStreamKey 记录某个 session 当前正在运行的 active stream。
func GenerateSessionActiveStreamKey(sessionID string) string {
	return fmt.Sprintf("ai:session:active-stream:%s", sessionID)
}

// GenerateStreamMetaKey 保存某个 active stream 的恢复元数据。
func GenerateStreamMetaKey(streamID string) string {
	return fmt.Sprintf("ai:stream:meta:%s", streamID)
}

// GenerateStreamChunksKey 保存某个 active stream 最近一段 chunk 环形缓冲区。
func GenerateStreamChunksKey(streamID string) string {
	return fmt.Sprintf("ai:stream:chunks:%s", streamID)
}

// GenerateStreamSnapshotKey 保存某个 active stream 的整段文本快照。
func GenerateStreamSnapshotKey(streamID string) string {
	return fmt.Sprintf("ai:stream:snapshot:%s", streamID)
}

// GenerateStreamStopSignalKey 保存某个 active stream 的显式 stop 信号。
func GenerateStreamStopSignalKey(streamID string) string {
	return fmt.Sprintf("ai:stream:stop:%s", streamID)
}
