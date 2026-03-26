package main

import (
	"GopherAI/common/mysql"
	"GopherAI/common/rabbitmq"
	"GopherAI/common/redis"
	"GopherAI/config"
	"GopherAI/router"
	"fmt"
	"log"
)

func StartServer(addr string, port int) error {
	r := router.InitRouter()
	// 目前静态资源映射不在这轮改造范围内，先保持现状。
	// r.Static(config.GetConfig().HttpFilePath, config.GetConfig().MusicFilePath)
	return r.Run(fmt.Sprintf("%s:%d", addr, port))
}

func main() {
	conf := config.GetConfig()
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port

	// 这里只初始化基础设施本身，不再在启动期把全部历史消息预热进本地 helper。
	// 会话上下文改为在真正访问某个 session 时再按需恢复和对齐。
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

	err := StartServer(host, port)
	if err != nil {
		panic(err)
	}
}
