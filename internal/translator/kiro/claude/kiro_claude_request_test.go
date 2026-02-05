package claude

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildKiroPayload_ThinkingBudgetMapsToMaxThinkingLength verifies that when
// thinking.budget_tokens is provided in the Claude request, it is mapped to
// <max_thinking_length> in the injected thinking tags.
func TestBuildKiroPayload_ThinkingBudgetMapsToMaxThinkingLength(t *testing.T) {
	// Claude request with thinking.budget_tokens = 32000
	claudeBody := []byte(`{
		"model": "claude-opus-4-5",
		"max_tokens": 4096,
		"thinking": {
			"type": "enabled",
			"budget_tokens": 32000
		},
		"messages": [
			{"role": "user", "content": "Hello, please think about this."}
		]
	}`)

	payload, thinkingEnabled := BuildKiroPayload(claudeBody, "kiro-model", "", "CLI", false, false, nil, nil)

	if !thinkingEnabled {
		t.Fatal("Expected thinking to be enabled")
	}

	var kiroPayload KiroPayload
	if err := json.Unmarshal(payload, &kiroPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	// Extract the content which should contain the system prompt with thinking tags
	content := kiroPayload.ConversationState.CurrentMessage.UserInputMessage.Content

	// Check that max_thinking_length is set to 32000 (from budget_tokens)
	if !strings.Contains(content, "<max_thinking_length>32000</max_thinking_length>") {
		t.Errorf("Expected max_thinking_length to be 32000, but got content: %s", content)
	}

	// Also verify thinking_mode is enabled
	if !strings.Contains(content, "<thinking_mode>enabled</thinking_mode>") {
		t.Error("Expected thinking_mode to be enabled")
	}
}

// TestBuildKiroPayload_ThinkingNoBudgetUsesDefault verifies that when thinking
// is enabled but no budget_tokens is provided, the default value of 16000 is used.
func TestBuildKiroPayload_ThinkingNoBudgetUsesDefault(t *testing.T) {
	// Claude request with thinking enabled but no budget_tokens
	claudeBody := []byte(`{
		"model": "claude-opus-4-5",
		"max_tokens": 4096,
		"thinking": {
			"type": "enabled"
		},
		"messages": [
			{"role": "user", "content": "Hello, please think about this."}
		]
	}`)

	payload, thinkingEnabled := BuildKiroPayload(claudeBody, "kiro-model", "", "CLI", false, false, nil, nil)

	if !thinkingEnabled {
		t.Fatal("Expected thinking to be enabled")
	}

	var kiroPayload KiroPayload
	if err := json.Unmarshal(payload, &kiroPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	content := kiroPayload.ConversationState.CurrentMessage.UserInputMessage.Content

	// Check that max_thinking_length defaults to 16000
	if !strings.Contains(content, "<max_thinking_length>16000</max_thinking_length>") {
		t.Errorf("Expected max_thinking_length to default to 16000, but got content: %s", content)
	}

	if !strings.Contains(content, "<thinking_mode>enabled</thinking_mode>") {
		t.Error("Expected thinking_mode to be enabled")
	}
}

// TestBuildKiroPayload_ThinkingZeroBudgetUsesDefault verifies that when
// budget_tokens is set to 0 or negative, the default value of 16000 is used.
func TestBuildKiroPayload_ThinkingZeroBudgetUsesDefault(t *testing.T) {
	// Claude request with budget_tokens = 0
	claudeBody := []byte(`{
		"model": "claude-opus-4-5",
		"max_tokens": 4096,
		"thinking": {
			"type": "enabled",
			"budget_tokens": 0
		},
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`)

	_, thinkingEnabled := BuildKiroPayload(claudeBody, "kiro-model", "", "CLI", false, false, nil, nil)

	// Note: budget_tokens = 0 disables thinking per checkThinkingMode logic
	if thinkingEnabled {
		t.Fatal("Expected thinking to be disabled when budget_tokens = 0")
	}
}

// TestBuildKiroPayload_SkipsInjectionWhenTagsPresent verifies that when the
// system prompt already contains thinking tags (e.g., from AMP/Cursor client),
// we skip injection to avoid duplicates.
func TestBuildKiroPayload_SkipsInjectionWhenTagsPresent(t *testing.T) {
	// Claude request with thinking tags already in system prompt
	claudeBody := []byte(`{
		"model": "claude-opus-4-5",
		"max_tokens": 4096,
		"system": "<thinking_mode>interleaved</thinking_mode>\n<max_thinking_length>20000</max_thinking_length>\nYou are a helpful assistant.",
		"thinking": {
			"type": "enabled",
			"budget_tokens": 32000
		},
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`)

	payload, thinkingEnabled := BuildKiroPayload(claudeBody, "kiro-model", "", "CLI", false, false, nil, nil)

	if !thinkingEnabled {
		t.Fatal("Expected thinking to be enabled")
	}

	var kiroPayload KiroPayload
	if err := json.Unmarshal(payload, &kiroPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	content := kiroPayload.ConversationState.CurrentMessage.UserInputMessage.Content

	// Count occurrences of thinking_mode tags - should only be 1 (from original system prompt)
	thinkingModeCount := strings.Count(content, "<thinking_mode>")
	if thinkingModeCount != 1 {
		t.Errorf("Expected exactly 1 <thinking_mode> tag (no injection), got %d. Content: %s", thinkingModeCount, content)
	}

	// The original max_thinking_length should be preserved (20000, not overridden to 32000)
	if !strings.Contains(content, "<max_thinking_length>20000</max_thinking_length>") {
		t.Error("Expected original max_thinking_length (20000) to be preserved")
	}
	if strings.Contains(content, "<max_thinking_length>32000</max_thinking_length>") {
		t.Error("Should not inject new max_thinking_length when tags already present")
	}
}

// TestBuildKiroPayload_ThinkingViaHeader verifies that thinking can be enabled
// via Anthropic-Beta header and uses the default max_thinking_length.
func TestBuildKiroPayload_ThinkingViaHeader(t *testing.T) {
	claudeBody := []byte(`{
		"model": "claude-opus-4-5",
		"max_tokens": 4096,
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`)

	// Create headers with Anthropic-Beta
	headers := make(map[string][]string)
	headers["Anthropic-Beta"] = []string{"interleaved-thinking-2025-05-14"}

	payload, thinkingEnabled := BuildKiroPayload(claudeBody, "kiro-model", "", "CLI", false, false, headers, nil)

	if !thinkingEnabled {
		t.Fatal("Expected thinking to be enabled via header")
	}

	var kiroPayload KiroPayload
	if err := json.Unmarshal(payload, &kiroPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	content := kiroPayload.ConversationState.CurrentMessage.UserInputMessage.Content

	// Should use default 16000 since no budget_tokens in body
	if !strings.Contains(content, "<max_thinking_length>16000</max_thinking_length>") {
		t.Errorf("Expected max_thinking_length to default to 16000 with header-based thinking, got: %s", content)
	}
}

// TestBuildKiroPayload_NoThinkingNoInjection verifies that when thinking is not
// enabled, no thinking tags are injected.
func TestBuildKiroPayload_NoThinkingNoInjection(t *testing.T) {
	claudeBody := []byte(`{
		"model": "claude-opus-4-5",
		"max_tokens": 4096,
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`)

	payload, thinkingEnabled := BuildKiroPayload(claudeBody, "kiro-model", "", "CLI", false, false, nil, nil)

	if thinkingEnabled {
		t.Fatal("Expected thinking to be disabled")
	}

	var kiroPayload KiroPayload
	if err := json.Unmarshal(payload, &kiroPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	content := kiroPayload.ConversationState.CurrentMessage.UserInputMessage.Content

	// Should not contain any thinking tags
	if strings.Contains(content, "<thinking_mode>") {
		t.Error("Should not inject thinking_mode when thinking is disabled")
	}
	if strings.Contains(content, "<max_thinking_length>") {
		t.Error("Should not inject max_thinking_length when thinking is disabled")
	}
}

// TestBuildKiroPayload_ThinkingWithSystemPrompt verifies that thinking tags
// are properly injected before the system prompt.
func TestBuildKiroPayload_ThinkingWithSystemPrompt(t *testing.T) {
	claudeBody := []byte(`{
		"model": "claude-opus-4-5",
		"max_tokens": 4096,
		"system": "You are a helpful assistant.",
		"thinking": {
			"type": "enabled",
			"budget_tokens": 25000
		},
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`)

	payload, thinkingEnabled := BuildKiroPayload(claudeBody, "kiro-model", "", "CLI", false, false, nil, nil)

	if !thinkingEnabled {
		t.Fatal("Expected thinking to be enabled")
	}

	var kiroPayload KiroPayload
	if err := json.Unmarshal(payload, &kiroPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	content := kiroPayload.ConversationState.CurrentMessage.UserInputMessage.Content

	// Verify thinking tags appear before system prompt
	thinkingModeIdx := strings.Index(content, "<thinking_mode>enabled</thinking_mode>")
	maxThinkingIdx := strings.Index(content, "<max_thinking_length>25000</max_thinking_length>")
	systemPromptIdx := strings.Index(content, "You are a helpful assistant.")

	if thinkingModeIdx == -1 || maxThinkingIdx == -1 {
		t.Fatal("Expected thinking tags to be present")
	}
	if systemPromptIdx == -1 {
		t.Fatal("Expected system prompt to be present")
	}

	// Thinking tags should come before system prompt
	if thinkingModeIdx > systemPromptIdx || maxThinkingIdx > systemPromptIdx {
		t.Error("Thinking tags should be injected before system prompt")
	}
}
