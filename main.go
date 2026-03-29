package main

import (
	"GopherAI/common/mysql"
	"GopherAI/common/rabbitmq"
	"GopherAI/common/redis"
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
	// 1. server：只跑 API；
	// 2. worker：跑全部后台 worker，包括向量化消费和补偿扫描；
	// 3. all：开发环境下一把启动 API + 全部 worker。
	role := flag.String("role", "server", "process role: server, worker, all")
	flag.Parse()

	conf := config.GetConfig()
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

	rabbitmq.InitRabbitMQ()
	log.Println("rabbitmq init success")

	// 当前先统一使用一个根 context 管理进程级生命周期。
	// 后续如果要做优雅停机，可以在这里对接 signal 和 cancel。
	ctx := context.Background()
	switch *role {
	case "server":
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
