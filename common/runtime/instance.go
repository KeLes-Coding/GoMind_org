package runtime

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

type InstanceInfo struct {
	ID            string
	Role          string
	OwnerEligible bool
}

var (
	instanceMu   sync.RWMutex
	instanceInfo = InstanceInfo{
		ID:            buildDefaultInstanceID(),
		Role:          "server",
		OwnerEligible: true,
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

// InitInstanceInfo 初始化当前进程的实例标识和角色信息。
func InitInstanceInfo(role string) {
	instanceMu.Lock()
	defer instanceMu.Unlock()

	if role == "" {
		role = "server"
	}
	instanceInfo.Role = role
	instanceInfo.OwnerEligible = role == "server" || role == "all"
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
