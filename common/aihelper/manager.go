package aihelper

import (
	"context"
	"sync"
)

var ctx = context.Background()

// AIHelperManager 负责管理 user -> session -> helper 的运行时映射关系。
// 它是运行时缓存，不应该再承担会话列表或聊天历史的真相来源职责。
type AIHelperManager struct {
	helpers map[string]map[string]*AIHelper
	mu      sync.RWMutex
}

// NewAIHelperManager 创建新的管理器实例。
func NewAIHelperManager() *AIHelperManager {
	return &AIHelperManager{
		helpers: make(map[string]map[string]*AIHelper),
	}
}

// GetOrCreateAIHelper 获取或创建某个用户某个会话对应的 helper。
func (m *AIHelperManager) GetOrCreateAIHelper(userName string, sessionID string, modelType string, config RuntimeConfig) (*AIHelper, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		userHelpers = make(map[string]*AIHelper)
		m.helpers[userName] = userHelpers
	}

	helper, exists := userHelpers[sessionID]
	if exists && helper.MatchesSelection(config.SelectionSignature(modelType)) {
		return helper, nil
	}

	factory := GetGlobalFactory()
	helper, err := factory.CreateAIHelper(ctx, modelType, sessionID, config)
	if err != nil {
		return nil, err
	}

	userHelpers[sessionID] = helper
	return helper, nil
}

// GetAIHelper 读取指定用户、指定会话的 helper。
func (m *AIHelperManager) GetAIHelper(userName string, sessionID string) (*AIHelper, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return nil, false
	}

	helper, exists := userHelpers[sessionID]
	return helper, exists
}

// SetAIHelper 把一个已经准备好的 helper 放入管理器。
// 这个方法用于“先从数据库回放历史，再缓存 helper”这一类场景。
func (m *AIHelperManager) SetAIHelper(userName string, sessionID string, helper *AIHelper) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		userHelpers = make(map[string]*AIHelper)
		m.helpers[userName] = userHelpers
	}

	userHelpers[sessionID] = helper
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
	if len(userHelpers) == 0 {
		delete(m.helpers, userName)
	}
}

// GetUserSessions 返回当前进程内缓存过的 sessionID 列表。
// 这个方法保留给运行时观察使用，不再建议作为业务接口真相来源。
func (m *AIHelperManager) GetUserSessions(userName string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

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

var globalManager *AIHelperManager
var once sync.Once

// GetGlobalManager 返回全局单例管理器。
func GetGlobalManager() *AIHelperManager {
	once.Do(func() {
		globalManager = NewAIHelperManager()
	})
	return globalManager
}
