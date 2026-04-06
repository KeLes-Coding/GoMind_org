package mcpgateway

import "testing"

func TestIsToolAllowedRespectsBlocklistFirst(t *testing.T) {
	if isToolAllowed([]string{"get_weather"}, []string{"get_weather"}, "get_weather") {
		t.Fatal("expected blocklist to take precedence over allowlist")
	}
}

func TestTruncateToolResultAddsMarker(t *testing.T) {
	got := truncateToolResult("1234567890", 8)
	if got == "" {
		t.Fatal("expected truncated result to be non-empty")
	}
	if got == "1234567890" {
		t.Fatal("expected text to be truncated")
	}
}
