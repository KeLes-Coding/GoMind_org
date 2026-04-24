package session

import (
	"GopherAI/common/code"
	"GopherAI/model"
	"context"
	"testing"
	"time"
)

func TestActiveStreamRegistryRetainsTerminalTaskBriefly(t *testing.T) {
	baseTime := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	currentTime := baseTime

	registry := newActiveStreamRegistry()
	registry.nowFunc = func() time.Time {
		return currentTime
	}

	task := newActiveStreamTask("tester", "session-retain", "stream-retain", "message-retain", func() {})
	task.finish(model.StreamStatusCompleted)
	registry.register(task)
	registry.markRetained(task, 5*time.Second)

	if got := registry.getByStreamID("stream-retain"); got == nil {
		t.Fatal("expected retained terminal task to remain accessible before ttl expires")
	}

	currentTime = baseTime.Add(6 * time.Second)
	if got := registry.getByStreamID("stream-retain"); got != nil {
		t.Fatal("expected retained terminal task to be evicted after ttl expires")
	}
}

func TestActiveStreamRegistryStopIgnoresTerminalTask(t *testing.T) {
	registry := newActiveStreamRegistry()
	task := newActiveStreamTask("tester", "session-stop", "stream-stop", "message-stop", context.CancelFunc(func() {}))
	task.finish(model.StreamStatusCompleted)
	registry.register(task)
	registry.markRetained(task, 5*time.Second)

	if _, code_ := registry.stop("tester", "session-stop"); code_ != code.CodeChatNotRunning {
		t.Fatalf("expected terminal task stop to return CodeChatNotRunning, got %d", code_)
	}
}
