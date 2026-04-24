package aihelper

import (
	"testing"
	"time"
)

func TestAIHelperManagerExpiresExecutionCacheEntry(t *testing.T) {
	baseTime := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	currentTime := baseTime

	manager := NewAIHelperManager()
	manager.helperTTL = 2 * time.Second
	manager.nowFunc = func() time.Time {
		return currentTime
	}

	helper := NewAIHelper(stubAIModel{}, "session-ttl", "selection-ttl")
	manager.SetAIHelper("tester", "session-ttl", helper)

	if _, exists := manager.GetAIHelper("tester", "session-ttl"); !exists {
		t.Fatal("expected helper to exist before ttl expires")
	}

	currentTime = baseTime.Add(3 * time.Second)
	if _, exists := manager.GetAIHelper("tester", "session-ttl"); exists {
		t.Fatal("expected helper to be evicted after ttl expires")
	}
}

func TestAIHelperManagerRefreshesTTLOnAccess(t *testing.T) {
	baseTime := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	currentTime := baseTime

	manager := NewAIHelperManager()
	manager.helperTTL = 2 * time.Second
	manager.nowFunc = func() time.Time {
		return currentTime
	}

	helper := NewAIHelper(stubAIModel{}, "session-refresh", "selection-refresh")
	manager.SetAIHelper("tester", "session-refresh", helper)

	currentTime = baseTime.Add(1500 * time.Millisecond)
	if got, exists := manager.GetAIHelper("tester", "session-refresh"); !exists || got == nil {
		t.Fatal("expected helper to still exist on first refresh")
	}

	currentTime = baseTime.Add(3 * time.Second)
	if got, exists := manager.GetAIHelper("tester", "session-refresh"); !exists || got == nil {
		t.Fatal("expected helper access to refresh ttl")
	}
}
