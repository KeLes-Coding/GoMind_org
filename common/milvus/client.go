package milvus

import (
	"GopherAI/config"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

var (
	clientMu sync.Mutex
	client   *milvusclient.Client
)

// GetClient 返回进程级复用的 Milvus 客户端。
// 第一阶段先采用单例形式，避免在每次检索或写入时重复建立 gRPC 连接。
func GetClient(ctx context.Context) (*milvusclient.Client, error) {
	clientMu.Lock()
	defer clientMu.Unlock()

	if client != nil {
		return client, nil
	}

	cfg := config.GetConfig().MilvusConfig
	addr := strings.TrimSpace(cfg.Host)
	if addr == "" {
		addr = "127.0.0.1"
	}
	port := cfg.Port
	if port <= 0 {
		port = 19530
	}

	clientCfg := &milvusclient.ClientConfig{
		Address: fmt.Sprintf("%s:%d", addr, port),
		DBName:  strings.TrimSpace(cfg.Database),
	}
	if cfg.EnableAuth {
		clientCfg.Username = strings.TrimSpace(cfg.Username)
		clientCfg.Password = strings.TrimSpace(cfg.Password)
	}

	cli, err := milvusclient.New(ctx, clientCfg)
	if err != nil {
		return nil, err
	}

	client = cli
	return client, nil
}

// Close 用于在进程退出或测试场景中显式释放 Milvus 客户端。
func Close(ctx context.Context) error {
	clientMu.Lock()
	defer clientMu.Unlock()

	if client == nil {
		return nil
	}
	err := client.Close(ctx)
	client = nil
	return err
}
