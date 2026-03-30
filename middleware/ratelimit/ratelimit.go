package ratelimit

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type windowState struct {
	// 当前统计窗口的起始时间。
	windowStart time.Time
	// 当前窗口内已通过的请求次数。
	count int
}

type limiter struct {
	// 单个窗口内允许通过的最大请求数。
	limit int
	// 固定窗口大小，例如 1 分钟。
	window time.Duration

	// 保护 states，避免并发访问 map 产生竞态。
	mu sync.Mutex
	// 按 key 保存每个限流对象对应的窗口状态。
	states map[string]*windowState
}

// newLimiter 创建一个基于内存的固定窗口限流器。
func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{
		limit:  limit,
		window: window,
		states: make(map[string]*windowState),
	}
}

// allow 判断指定 key 当前是否还允许继续通过请求。
func (l *limiter) allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	for stateKey, state := range l.states {
		// 顺手清理过期窗口，避免服务长时间运行后 map 持续增长。
		if now.Sub(state.windowStart) > 2*l.window {
			delete(l.states, stateKey)
		}
	}

	state, ok := l.states[key]
	if !ok || now.Sub(state.windowStart) >= l.window {
		// 首次访问，或旧窗口已过期，则开启一个新的统计窗口。
		l.states[key] = &windowState{
			windowStart: now,
			count:       1,
		}
		return true
	}

	if state.count >= l.limit {
		return false
	}

	// 仍在当前窗口内且未达到阈值，请求计数加一并放行。
	state.count++
	return true
}

var (
	// 当前版本先使用单机内存限流；如果后续部署为多实例，再迁移到 Redis 等集中式方案。
	loginIPLimiter   = newLimiter(10, time.Minute)
	captchaIPLimiter = newLimiter(5, time.Minute)
	chatIPLimiter    = newLimiter(30, time.Minute)
	chatUserLimiter  = newLimiter(12, time.Minute)
)

// reject 返回“请求过多”响应，并中止后续中间件和处理函数。
func reject(c *gin.Context) {
	res := new(controller.Response)
	c.JSON(http.StatusOK, res.CodeOf(code.CodeTooManyRequests))
	c.Abort()
}

// LimitLoginByIP 按客户端 IP 限制登录接口访问频率。
func LimitLoginByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !loginIPLimiter.allow("login:" + c.ClientIP()) {
			reject(c)
			return
		}
		c.Next()
	}
}

// LimitCaptchaByIP 按客户端 IP 限制验证码接口访问频率。
func LimitCaptchaByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !captchaIPLimiter.allow("captcha:" + c.ClientIP()) {
			reject(c)
			return
		}
		c.Next()
	}
}

// LimitChatByIP 按客户端 IP 限制聊天接口访问频率。
func LimitChatByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !chatIPLimiter.allow("chat:" + c.ClientIP()) {
			reject(c)
			return
		}
		c.Next()
	}
}

// LimitChatByUser 按登录用户名限制聊天接口访问频率。
func LimitChatByUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userName := c.GetString("userName")
		if userName == "" {
			reject(c)
			return
		}
		if !chatUserLimiter.allow("chat-user:" + userName) {
			reject(c)
			return
		}
		c.Next()
	}
}
