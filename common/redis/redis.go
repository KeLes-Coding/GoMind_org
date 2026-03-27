package redis

import (
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
		redisAvailable.Store(false)
		return err
	}

	// 只要启动期探活成功，就允许业务优先走 Redis。
	redisAvailable.Store(true)
	return nil
}

func IsAvailable() bool {
	return Rdb != nil && redisAvailable.Load()
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
		redisAvailable.Store(false)
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
		redisAvailable.Store(false)
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
		redisAvailable.Store(false)
		return err
	}
	return nil
}

func InitRedisIndex(ctx context.Context, filename string, dimension int) error {
	if !IsAvailable() {
		return fmt.Errorf("redis unavailable")
	}

	indexName := GenerateIndexName(filename)
	_, err := Rdb.Do(ctx, "FT.INFO", indexName).Result()
	if err == nil {
		fmt.Println("redis index already exists, skip create")
		return nil
	}

	if !strings.Contains(err.Error(), "Unknown index name") {
		redisAvailable.Store(false)
		return fmt.Errorf("check redis index failed: %w", err)
	}

	fmt.Println("creating redis index")

	prefix := GenerateIndexNamePrefix(filename)
	createArgs := []interface{}{
		"FT.CREATE", indexName,
		"ON", "HASH",
		"PREFIX", "1", prefix,
		"SCHEMA",
		"content", "TEXT",
		"metadata", "TEXT",
		"vector", "VECTOR", "FLAT",
		"6",
		"TYPE", "FLOAT32",
		"DIM", dimension,
		"DISTANCE_METRIC", "COSINE",
	}

	if err := Rdb.Do(ctx, createArgs...).Err(); err != nil {
		redisAvailable.Store(false)
		return fmt.Errorf("create redis index failed: %w", err)
	}

	fmt.Println("redis index created")
	return nil
}

func DeleteRedisIndex(ctx context.Context, filename string) error {
	if !IsAvailable() {
		return fmt.Errorf("redis unavailable")
	}

	indexName := GenerateIndexName(filename)
	if err := Rdb.Do(ctx, "FT.DROPINDEX", indexName).Err(); err != nil {
		redisAvailable.Store(false)
		return fmt.Errorf("delete redis index failed: %w", err)
	}

	fmt.Println("redis index deleted")
	return nil
}
