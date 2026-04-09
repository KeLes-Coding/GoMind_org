package redis

import "testing"

func TestSelectPreferredSessionOwnerHRWStable(t *testing.T) {
	instances := []ChatInstanceMeta{
		{InstanceID: "chat-a", Weight: 100},
		{InstanceID: "chat-b", Weight: 100},
		{InstanceID: "chat-c", Weight: 100},
	}
	sessionID := "session-123"

	first := selectPreferredSessionOwnerHRW(sessionID, instances)
	if first == "" {
		t.Fatal("expected non-empty preferred owner")
	}

	for i := 0; i < 10; i++ {
		next := selectPreferredSessionOwnerHRW(sessionID, instances)
		if next != first {
			t.Fatalf("expected stable preferred owner, got %s then %s", first, next)
		}
	}
}

func TestSelectPreferredSessionOwnerHRWEmpty(t *testing.T) {
	if owner := selectPreferredSessionOwnerHRW("session-123", nil); owner != "" {
		t.Fatalf("expected empty owner for empty instance list, got %s", owner)
	}
}

func TestDecodeChatInstanceMetaBackwardCompatible(t *testing.T) {
	meta := decodeChatInstanceMeta("chat-a", "server")
	if meta.InstanceID != "chat-a" {
		t.Fatalf("expected instance id chat-a, got %s", meta.InstanceID)
	}
	if meta.Role != "server" {
		t.Fatalf("expected role server, got %s", meta.Role)
	}
	if meta.Weight != 100 {
		t.Fatalf("expected default weight 100, got %d", meta.Weight)
	}
}
