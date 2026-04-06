package main

import (
	"GopherAI/common/applog"
	commonMilvus "GopherAI/common/milvus"
	"GopherAI/common/mysql"
	"GopherAI/common/observability"
	"GopherAI/common/rabbitmq"
	"GopherAI/common/redis"
	rt "GopherAI/common/runtime"
	"GopherAI/common/vectorruntime"
	"GopherAI/config"
	"GopherAI/router"
	"GopherAI/worker"
	"context"
	"flag"
	"fmt"
	"log"
)

func StartServer(addr string, port int) error {
	r := router.InitRouter()
	return r.Run(fmt.Sprintf("%s:%d", addr, port))
}

func main() {
	// role 是这一轮加入的最小进程角色拆分：
	// 1. server：跑 API，并顺带启动聊天链路必需的轻量补偿 worker；
	// 2. worker：跑全部后台 worker，包括向量化消费和补偿扫描；
	// 3. all：开发环境下一把启动 API + 全部 worker。
	role := flag.String("role", "server", "process role: server, worker, all")
	flag.Parse()
	rt.InitInstanceInfo(*role)

	conf := config.GetConfig()
	observability.RecordRAGStoreMode(vectorruntime.CurrentStoreMode())
	if err := applog.Setup(applog.Config{
		Path:      conf.LogConfig.Path,
		MaxSizeMB: conf.LogConfig.MaxSizeMB,
	}); err != nil {
		log.Println("applog setup degraded, fallback to default stderr:", err)
	}
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port

	if err := mysql.InitMysql(); err != nil {
		log.Println("InitMysql error , " + err.Error())
		return
	}

	if err := redis.Init(); err != nil {
		log.Println("redis init degraded, fallback to database for captcha flow:", err)
	} else {
		log.Println("redis init success")
	}

	ctx := context.Background()
	if err := commonMilvus.Init(ctx); err != nil {
		log.Println("milvus init degraded, RAG vector store unavailable:", err)
	} else {
		log.Println("milvus init success")
	}

	rabbitmq.InitRabbitMQ()
	log.Println("rabbitmq init success")

	// 当前先统一使用一个根 context 管理进程级生命周期。
	// 后续如果要做优雅停机，可以在这里对接 signal 和 cancel。
	redis.StartChatInstanceHeartbeat(ctx)
	switch *role {
	case "server":
		// server 模式也需要跑聊天链路的 relay / persisted_version 补偿。
		// 否则如果只启动 API，不启动独立 worker，消息 outbox 会无人处理。
		go func() {
			if err := worker.StartMessageOutboxRelayWorker(ctx); err != nil {
				log.Printf("Message outbox relay worker error: %v", err)
			}
		}()
		go func() {
			if err := worker.StartSessionPersistenceCompensationWorker(ctx); err != nil {
				log.Printf("Session persistence compensation worker error: %v", err)
			}
		}()
		if err := StartServer(host, port); err != nil {
			panic(err)
		}
	case "worker":
		worker.StartAllWorkers(ctx)
		select {}
	case "all":
		worker.StartAllWorkers(ctx)
		if err := StartServer(host, port); err != nil {
			panic(err)
		}
	default:
		log.Fatalf("unsupported role: %s", *role)
	}
}
