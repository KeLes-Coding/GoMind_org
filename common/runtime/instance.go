package runtime

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

type InstanceInfo struct {
	ID            string
	Role          string
	OwnerEligible bool
	OwnerWeight   int
}

var (
	instanceMu   sync.RWMutex
	instanceInfo = InstanceInfo{
		ID:            buildDefaultInstanceID(),
		Role:          "server",
		OwnerEligible: true,
		OwnerWeight:   100,
	}
)

func buildDefaultInstanceID() string {
	hostName, err := os.Hostname()
	if err != nil || hostName == "" {
		hostName = "unknown-host"
	}
	// 默认实例 ID 带上 hostname / pid / 随机后缀，避免同机多进程时互相冲突。
	return fmt.Sprintf("%s-%d-%d-%s", hostName, os.Getpid(), time.Now().Unix(), uuid.NewString()[:8])
}

func parseOwnerWeightFromEnv() int {
	weightText := os.Getenv("CHAT_INSTANCE_WEIGHT")
	if weightText == "" {
		return 100
	}
	weight, err := strconv.Atoi(weightText)
	if err != nil || weight <= 0 {
		return 100
	}
	return weight
}

// InitInstanceInfo 初始化当前进程的实例标识和角色信息。
func InitInstanceInfo(role string) {
	instanceMu.Lock()
	defer instanceMu.Unlock()

	if role == "" {
		role = "server"
	}
	instanceInfo.Role = role
	instanceInfo.OwnerEligible = role == "server" || role == "all"
	// OwnerWeight 用于会话路由的加权 HRW 选主。
	// 默认值为 100，后续可通过 CHAT_INSTANCE_WEIGHT 做实例级权重调节。
	instanceInfo.OwnerWeight = parseOwnerWeightFromEnv()
}

// CurrentInstanceID 返回当前进程实例 ID。
func CurrentInstanceID() string {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return instanceInfo.ID
}

// CurrentRole 返回当前进程角色。
func CurrentRole() string {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return instanceInfo.Role
}

// IsOwnerEligible 表示当前实例是否参与聊天 owner 选举。
func IsOwnerEligible() bool {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return instanceInfo.OwnerEligible
}

// CurrentOwnerWeight 返回当前实例参与会话选主时使用的权重。
func CurrentOwnerWeight() int {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return instanceInfo.OwnerWeight
}
