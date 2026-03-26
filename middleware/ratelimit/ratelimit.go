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
	windowStart time.Time
	count       int
}

type limiter struct {
	limit  int
	window time.Duration

	mu     sync.Mutex
	states map[string]*windowState
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{
		limit:  limit,
		window: window,
		states: make(map[string]*windowState),
	}
}

func (l *limiter) allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	for stateKey, state := range l.states {
		// 惰性清理过期窗口，避免长时间运行后 map 无限制增长。
		if now.Sub(state.windowStart) > 2*l.window {
			delete(l.states, stateKey)
		}
	}

	state, ok := l.states[key]
	if !ok || now.Sub(state.windowStart) >= l.window {
		l.states[key] = &windowState{
			windowStart: now,
			count:       1,
		}
		return true
	}

	if state.count >= l.limit {
		return false
	}

	state.count++
	return true
}

var (
	// 这一版先做单机内存限流；如果后续是多实例部署，再迁到 Redis。
	loginIPLimiter   = newLimiter(10, time.Minute)
	captchaIPLimiter = newLimiter(5, time.Minute)
	chatIPLimiter    = newLimiter(30, time.Minute)
	chatUserLimiter  = newLimiter(12, time.Minute)
)

func reject(c *gin.Context) {
	res := new(controller.Response)
	c.JSON(http.StatusOK, res.CodeOf(code.CodeTooManyRequests))
	c.Abort()
}

func LimitLoginByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !loginIPLimiter.allow("login:" + c.ClientIP()) {
			reject(c)
			return
		}
		c.Next()
	}
}

func LimitCaptchaByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !captchaIPLimiter.allow("captcha:" + c.ClientIP()) {
			reject(c)
			return
		}
		c.Next()
	}
}

func LimitChatByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !chatIPLimiter.allow("chat:" + c.ClientIP()) {
			reject(c)
			return
		}
		c.Next()
	}
}

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
