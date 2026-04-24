package aihelper

import (
	"GopherAI/common/observability"
	"sync"
	"time"
)

const (
	// defaultHelperExecutionCacheTTL 控制本地 helper 作为 execution cache 的默认存活时间。
	// 第五阶段开始，它只服务于同实例短时复用，不再作为跨请求恢复真相源。
	defaultHelperExecutionCacheTTL = 90 * time.Second
)

type helperCacheEntry struct {
	helper    *AIHelper
	expiresAt time.Time
}

// AIHelperManager 负责管理 user -> session -> helper 的运行时映射关系。
// 它是运行时缓存，不应该再承担会话列表或聊天历史的真相来源职责。
type AIHelperManager struct {
	helpers   map[string]map[string]*helperCacheEntry
	helperTTL time.Duration
	nowFunc   func() time.Time
	mu        sync.RWMutex
}

// NewAIHelperManager 创建新的管理器实例。
func NewAIHelperManager() *AIHelperManager {
	return &AIHelperManager{
		helpers:   make(map[string]map[string]*helperCacheEntry),
		helperTTL: defaultHelperExecutionCacheTTL,
		nowFunc:   time.Now,
	}
}

// GetAIHelper 读取指定用户、指定会话的 helper。
func (m *AIHelperManager) GetAIHelper(userName string, sessionID string) (*AIHelper, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return nil, false
	}

	entry, exists := userHelpers[sessionID]
	if !exists || entry == nil || entry.helper == nil {
		return nil, false
	}
	entry.expiresAt = m.nextExpiryLocked()
	return entry.helper, true
}

// SetAIHelper 把一个已经准备好的 helper 放入管理器。
// 这个方法用于“先从数据库回放历史，再缓存 helper”这一类场景。
func (m *AIHelperManager) SetAIHelper(userName string, sessionID string, helper *AIHelper) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		userHelpers = make(map[string]*helperCacheEntry)
		m.helpers[userName] = userHelpers
	}

	userHelpers[sessionID] = &helperCacheEntry{
		helper:    helper,
		expiresAt: m.nextExpiryLocked(),
	}
}

// RemoveAIHelper 移除指定会话对应的 helper。
func (m *AIHelperManager) RemoveAIHelper(userName string, sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return
	}

	delete(userHelpers, sessionID)
	observability.RecordHelperExecutionRelease()
	if len(userHelpers) == 0 {
		delete(m.helpers, userName)
	}
}

// GetUserSessions 返回当前进程内缓存过的 sessionID 列表。
// 这个方法保留给运行时观察使用，不再建议作为业务接口真相来源。
func (m *AIHelperManager) GetUserSessions(userName string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return []string{}
	}

	sessionIDs := make([]string, 0, len(userHelpers))
	for sessionID := range userHelpers {
		sessionIDs = append(sessionIDs, sessionID)
	}

	return sessionIDs
}

func (m *AIHelperManager) nextExpiryLocked() time.Time {
	return m.nowFunc().Add(m.helperTTL)
}

func (m *AIHelperManager) cleanupExpiredLocked() {
	now := m.nowFunc()
	for userName, userHelpers := range m.helpers {
		for sessionID, entry := range userHelpers {
			if entry == nil || entry.helper == nil || (!entry.expiresAt.IsZero() && !entry.expiresAt.After(now)) {
				delete(userHelpers, sessionID)
				observability.RecordHelperExecutionRelease()
			}
		}
		if len(userHelpers) == 0 {
			delete(m.helpers, userName)
		}
	}
}

var globalManager *AIHelperManager
var once sync.Once

// GetGlobalManager 返回全局单例管理器。
func GetGlobalManager() *AIHelperManager {
	once.Do(func() {
		globalManager = NewAIHelperManager()
	})
	return globalManager
}
