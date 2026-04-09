package redis

import (
	"GopherAI/model"
	"context"
	"encoding/json"
	"strings"
	"time"

	redisCli "github.com/redis/go-redis/v9"
)

const (
	// activeStreamMetaTTL 让终态流在短时间内仍可被恢复接口查询到，
	// 便于客户端在网络抖动后补最后一段 backlog 并拿到最终状态。
	activeStreamMetaTTL = 10 * time.Minute
	// activeStreamChunksTTL 比 meta 略短一些，只承担短续传窗口的 chunk 回放。
	activeStreamChunksTTL = 5 * time.Minute
	// activeStreamSnapshotTTL 保留略长一点，便于缓冲区覆盖后仍能用 snapshot 兜底恢复。
	activeStreamSnapshotTTL = 10 * time.Minute
	activeStreamStopSignalTTL = 5 * time.Minute
)

// SaveSessionActiveStream 把 session -> stream 的映射写入 Redis。
// 这让重连请求即使落到别的实例，也能先找到当前正在运行的流。
func SaveSessionActiveStream(ctx context.Context, sessionID string, streamID string) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(streamID) == "" || !IsAvailable() {
		return nil
	}
	if err := Rdb.Set(ctx, GenerateSessionActiveStreamKey(sessionID), streamID, activeStreamMetaTTL).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// GetSessionActiveStream 查询某个 session 当前 active stream 的 ID。
func GetSessionActiveStream(ctx context.Context, sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" || !IsAvailable() {
		return "", nil
	}
	result, err := Rdb.Get(ctx, GenerateSessionActiveStreamKey(sessionID)).Result()
	if err != nil {
		if err == redisCli.Nil {
			return "", nil
		}
		setAvailability(false)
		return "", err
	}
	return result, nil
}

// DeleteSessionActiveStream 删除 session 当前 active stream 的映射。
func DeleteSessionActiveStream(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" || !IsAvailable() {
		return nil
	}
	if err := Rdb.Del(ctx, GenerateSessionActiveStreamKey(sessionID)).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// SaveActiveStreamMeta 保存流式恢复所需的元数据。
func SaveActiveStreamMeta(ctx context.Context, meta *model.StreamResumeMeta) error {
	if meta == nil || strings.TrimSpace(meta.StreamID) == "" || !IsAvailable() {
		return nil
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := Rdb.Set(ctx, GenerateStreamMetaKey(meta.StreamID), string(payload), activeStreamMetaTTL).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// GetActiveStreamMeta 读取某个 active stream 的元数据。
func GetActiveStreamMeta(ctx context.Context, streamID string) (*model.StreamResumeMeta, error) {
	if strings.TrimSpace(streamID) == "" || !IsAvailable() {
		return nil, nil
	}
	result, err := Rdb.Get(ctx, GenerateStreamMetaKey(streamID)).Result()
	if err != nil {
		if err == redisCli.Nil {
			return nil, nil
		}
		setAvailability(false)
		return nil, err
	}

	var meta model.StreamResumeMeta
	if err := json.Unmarshal([]byte(result), &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// SaveActiveStreamSnapshot 保存当前累计文本快照。
func SaveActiveStreamSnapshot(ctx context.Context, snapshot *model.StreamSnapshot) error {
	if snapshot == nil || strings.TrimSpace(snapshot.StreamID) == "" || !IsAvailable() {
		return nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err := Rdb.Set(ctx, GenerateStreamSnapshotKey(snapshot.StreamID), string(payload), activeStreamSnapshotTTL).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// GetActiveStreamSnapshot 读取某个 active stream 的全文快照。
func GetActiveStreamSnapshot(ctx context.Context, streamID string) (*model.StreamSnapshot, error) {
	if strings.TrimSpace(streamID) == "" || !IsAvailable() {
		return nil, nil
	}
	result, err := Rdb.Get(ctx, GenerateStreamSnapshotKey(streamID)).Result()
	if err != nil {
		if err == redisCli.Nil {
			return nil, nil
		}
		setAvailability(false)
		return nil, err
	}

	var snapshot model.StreamSnapshot
	if err := json.Unmarshal([]byte(result), &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// AppendActiveStreamChunk 把最新 chunk 追加到 Redis 短期缓冲区里，并按 maxChunks 做裁剪。
// 这里故意只保留最近窗口，避免 Redis 长期累积全文。
func AppendActiveStreamChunk(ctx context.Context, streamID string, chunk *model.StreamChunkSnapshot, maxChunks int64) error {
	if chunk == nil || strings.TrimSpace(streamID) == "" || !IsAvailable() {
		return nil
	}
	payload, err := json.Marshal(chunk)
	if err != nil {
		return err
	}

	pipe := Rdb.TxPipeline()
	pipe.RPush(ctx, GenerateStreamChunksKey(streamID), string(payload))
	if maxChunks > 0 {
		pipe.LTrim(ctx, GenerateStreamChunksKey(streamID), -maxChunks, -1)
	}
	pipe.Expire(ctx, GenerateStreamChunksKey(streamID), activeStreamChunksTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// GetActiveStreamChunks 读取当前缓冲区内的 chunk 列表。
func GetActiveStreamChunks(ctx context.Context, streamID string) ([]model.StreamChunkSnapshot, error) {
	if strings.TrimSpace(streamID) == "" || !IsAvailable() {
		return nil, nil
	}
	values, err := Rdb.LRange(ctx, GenerateStreamChunksKey(streamID), 0, -1).Result()
	if err != nil {
		if err == redisCli.Nil {
			return nil, nil
		}
		setAvailability(false)
		return nil, err
	}

	chunks := make([]model.StreamChunkSnapshot, 0, len(values))
	for _, value := range values {
		var chunk model.StreamChunkSnapshot
		if err := json.Unmarshal([]byte(value), &chunk); err != nil {
			continue
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// SaveActiveStreamStopSignal 记录显式 stop 请求，供真正持有流式任务的实例轮询并取消。
func SaveActiveStreamStopSignal(ctx context.Context, streamID string) error {
	if strings.TrimSpace(streamID) == "" || !IsAvailable() {
		return nil
	}
	if err := Rdb.Set(ctx, GenerateStreamStopSignalKey(streamID), "1", activeStreamStopSignalTTL).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

// HasActiveStreamStopSignal 查询某个流是否收到了显式 stop。
func HasActiveStreamStopSignal(ctx context.Context, streamID string) (bool, error) {
	if strings.TrimSpace(streamID) == "" || !IsAvailable() {
		return false, nil
	}
	value, err := Rdb.Exists(ctx, GenerateStreamStopSignalKey(streamID)).Result()
	if err != nil {
		setAvailability(false)
		return false, err
	}
	return value > 0, nil
}

// DeleteActiveStreamStopSignal 删除 stop 信号，避免同一 stream 终态后继续误触发。
func DeleteActiveStreamStopSignal(ctx context.Context, streamID string) error {
	if strings.TrimSpace(streamID) == "" || !IsAvailable() {
		return nil
	}
	if err := Rdb.Del(ctx, GenerateStreamStopSignalKey(streamID)).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}
