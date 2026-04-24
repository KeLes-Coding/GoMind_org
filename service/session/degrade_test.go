package session

import (
	"GopherAI/common/code"
	"GopherAI/common/observability"
	"GopherAI/model"
	"context"
	"testing"
)

func TestTryTakeoverDetachedStreamResumeReturnsBusyWhenRedisDegraded(t *testing.T) {
	before := observability.SnapshotAI()

	meta := &model.StreamResumeMeta{
		StreamID:  "stream-degraded",
		SessionID: "session-degraded",
		Status:    model.StreamStatusDetached,
	}

	claimed, code_ := tryTakeoverDetachedStreamResume(context.Background(), meta)
	if claimed != nil {
		t.Fatal("expected no claimed meta when redis is degraded")
	}
	if code_ != code.CodeServerBusy {
		t.Fatalf("expected CodeServerBusy, got %d", code_)
	}

	after := observability.SnapshotAI()
	if after.StreamResumeRedisDegraded != before.StreamResumeRedisDegraded+1 {
		t.Fatalf("expected stream_resume_redis_degraded_total to increment by 1, before=%d after=%d", before.StreamResumeRedisDegraded, after.StreamResumeRedisDegraded)
	}
}
