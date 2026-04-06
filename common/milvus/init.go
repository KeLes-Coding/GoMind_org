package milvus

import (
	"context"
	"fmt"

	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// Init 在应用启动阶段初始化 Milvus 客户端和默认 collection。
// 这一步只负责“能连上 + collection 可用”，不承担数据迁移职责。
func Init(ctx context.Context) error {
	cli, err := GetClient(ctx)
	if err != nil {
		return fmt.Errorf("connect milvus failed: %w", err)
	}
	return EnsureCollection(ctx, cli, Dimension())
}

// EnsureCollection 确保默认 collection 已存在且已 load。
// 第一阶段采用“存在则复用，不存在则创建”的初始化策略。
func EnsureCollection(ctx context.Context, cli *milvusclient.Client, dimension int) error {
	collectionName := CollectionName()
	has, err := cli.HasCollection(ctx, milvusclient.NewHasCollectionOption(collectionName))
	if err != nil {
		return fmt.Errorf("check milvus collection failed: %w", err)
	}

	if !has {
		if err := cli.CreateCollection(ctx, NewCollectionOption(collectionName, dimension)); err != nil {
			return fmt.Errorf("create milvus collection failed: %w", err)
		}
	}

	loadTask, err := cli.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(collectionName))
	if err != nil {
		return fmt.Errorf("load milvus collection failed: %w", err)
	}
	if err := loadTask.Await(ctx); err != nil {
		return fmt.Errorf("await milvus collection load failed: %w", err)
	}

	return nil
}
