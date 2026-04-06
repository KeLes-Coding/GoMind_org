package redis

import (
	"GopherAI/common/observability"
	"GopherAI/config"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	redisCli "github.com/redis/go-redis/v9"
)

var Rdb *redisCli.Client

var ctx = context.Background()
var redisAvailable atomic.Bool

func setAvailability(available bool) {
	redisAvailable.Store(available)
	if available {
		observability.RecordRedisModeChange("normal")
		return
	}
	observability.RecordRedisModeChange("degraded")
}

func Init() error {
	conf := config.GetConfig()
	host := conf.RedisConfig.RedisHost
	port := conf.RedisConfig.RedisPort
	password := conf.RedisConfig.RedisPassword
	db := conf.RedisDb
	addr := host + ":" + strconv.Itoa(port)

	Rdb = redisCli.NewClient(&redisCli.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		Protocol: 2,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := Rdb.Ping(pingCtx).Err(); err != nil {
		setAvailability(false)
		return err
	}

	// 只要启动期探活成功，就允许业务优先走 Redis。
	setAvailability(true)
	setAvailability(true)
	return nil
}

func IsAvailable() bool {
	return Rdb != nil && redisAvailable.Load()
}

// CurrentMode 返回 Redis 当前运行模式，便于上层显式区分 normal / degraded。
func CurrentMode() string {
	if IsAvailable() {
		return "normal"
	}
	return "degraded"
}

// AcquireLock 获取分布式锁
func AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	if !IsAvailable() {
		return false, fmt.Errorf("redis not available")
	}
	// 使用 SET NX EX 实现分布式锁
	result, err := Rdb.SetNX(ctx, key, "locked", expiration).Result()
	return result, err
}

// ReleaseLock 释放分布式锁
func ReleaseLock(ctx context.Context, key string) error {
	if !IsAvailable() {
		return nil
	}
	return Rdb.Del(ctx, key).Err()
}

func SetCaptchaForEmail(email, captcha string) error {
	if !IsAvailable() {
		return fmt.Errorf("redis unavailable")
	}

	key := GenerateCaptcha(email)
	expire := 2 * time.Minute
	if err := Rdb.Set(ctx, key, captcha, expire).Err(); err != nil {
		// 运行期一旦发现 Redis 失败，后续请求直接走降级分支，避免每次阻塞等待。
		setAvailability(false)
		return err
	}
	return nil
}

func ValidateCaptchaForEmail(email, userInput string) (bool, error) {
	if !IsAvailable() {
		return false, fmt.Errorf("redis unavailable")
	}

	key := GenerateCaptcha(email)
	storedCaptcha, err := Rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redisCli.Nil {
			return false, nil
		}

		// 区分“验证码不存在”和“Redis 不可用”，让上层决定是否回退数据库。
		setAvailability(false)
		return false, err
	}

	return strings.EqualFold(storedCaptcha, userInput), nil
}

func DeleteCaptchaForEmail(email string) error {
	if !IsAvailable() {
		return fmt.Errorf("redis unavailable")
	}

	key := GenerateCaptcha(email)
	if err := Rdb.Del(ctx, key).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}

func InitRedisIndex(ctx context.Context, filename string, dimension int) error {
	return initRedisIndexWithPrefix(ctx, GenerateIndexName(filename), GenerateIndexNamePrefix(filename), dimension)
}

// InitUnifiedRAGIndex 初始化统一检索入口使用的共享索引。
// 这套索引和旧的“按文件一个索引”并存，便于新检索链路灰度切换时保留兼容兜底。
func InitUnifiedRAGIndex(ctx context.Context, dimension int) error {
	return initRedisIndexWithPrefix(ctx, GenerateUnifiedRAGIndexName(), GenerateUnifiedRAGIndexPrefix(), dimension)
}

func initRedisIndexWithPrefix(ctx context.Context, indexName, prefix string, dimension int) error {
	if !IsAvailable() {
		return fmt.Errorf("redis unavailable")
	}

	_, err := Rdb.Do(ctx, "FT.INFO", indexName).Result()
	if err == nil {
		fmt.Println("redis index already exists, skip create")
		return nil
	}

	if !isRedisIndexNotFoundError(err) {
		setAvailability(false)
		return fmt.Errorf("check redis index failed: %w", err)
	}

	fmt.Println("creating redis index")

	createArgs := []interface{}{
		"FT.CREATE", indexName,
		"ON", "HASH",
		"PREFIX", "1", prefix,
		"SCHEMA",
		"content", "TEXT",
		"metadata", "TEXT",
		// 这些字段是这轮 RAG 配套升级新增的“文件资产元数据”。
		// 设计目的有三个：
		// 1. 让检索结果能直接带回 file_id / version / file_name 等信息；
		// 2. 为后续统一索引 + 元数据过滤预留 schema；
		// 3. 让引用、排障、reindex 这些动作不再只依赖文件名推断。
		"file_id", "TAG",
		"file_version", "NUMERIC",
		"file_name", "TEXT",
		"storage_key", "TEXT",
		"content_sha256", "TAG",
		"chunk_id", "TAG",
		"chunk_index", "NUMERIC",
		"total_chunks", "NUMERIC",
		"owner_id", "NUMERIC",
		"kb_id", "TAG",
		"status", "TAG",
		"vector", "VECTOR", "FLAT",
		"6",
		"TYPE", "FLOAT32",
		"DIM", dimension,
		"DISTANCE_METRIC", "COSINE",
	}

	if err := Rdb.Do(ctx, createArgs...).Err(); err != nil {
		setAvailability(false)
		return fmt.Errorf("create redis index failed: %w", err)
	}

	fmt.Println("redis index created")
	return nil
}

func isRedisIndexNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown index name") || strings.Contains(msg, "no such index")
}

func DeleteRedisIndex(ctx context.Context, filename string) error {
	if !IsAvailable() {
		return fmt.Errorf("redis unavailable")
	}

	indexName := GenerateIndexName(filename)
	if err := Rdb.Do(ctx, "FT.DROPINDEX", indexName).Err(); err != nil {
		// 旧索引清理现在更多是“收尾治理动作”，并不适合把“索引本来就不存在”
		// 当成 Redis 整体不可用来处理，否则会误伤后续真正需要走 Redis 的链路。
		if isRedisIndexNotFoundError(err) {
			return nil
		}
		setAvailability(false)
		return fmt.Errorf("delete redis index failed: %w", err)
	}

	fmt.Println("redis index deleted")
	return nil
}

// SearchDocumentKeysByQuery 从指定索引里查出命中的 hash key 列表。
// 这里专门用于统一索引的文档治理场景，例如按 file_id 删除某份文件的所有 chunk。
func SearchDocumentKeysByQuery(ctx context.Context, indexName, query string, limit int) ([]string, error) {
	if !IsAvailable() {
		return nil, fmt.Errorf("redis unavailable")
	}
	if limit <= 0 {
		limit = 1000
	}

	result, err := Rdb.FTSearchWithArgs(ctx, indexName, query, &redisCli.FTSearchOptions{
		NoContent:      true,
		LimitOffset:    0,
		Limit:          limit,
		DialectVersion: 2,
	}).Result()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(result.Docs))
	for _, doc := range result.Docs {
		if doc.ID == "" {
			continue
		}
		keys = append(keys, doc.ID)
	}
	return keys, nil
}

// DeleteKeys 批量删除指定 hash key。
// 统一索引模式下，删除文件不再只是 drop 某个文件索引，而是需要显式清掉对应 chunk 文档。
func DeleteKeys(ctx context.Context, keys []string) error {
	if !IsAvailable() {
		return fmt.Errorf("redis unavailable")
	}
	if len(keys) == 0 {
		return nil
	}

	if err := Rdb.Del(ctx, keys...).Err(); err != nil {
		setAvailability(false)
		return err
	}
	return nil
}
