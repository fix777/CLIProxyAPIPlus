package executor

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func normalizedSSEItemID(t *testing.T, line []byte) string {
	t.Helper()
	if !bytes.HasPrefix(line, dataTag) {
		t.Fatalf("line does not start with data tag: %q", string(line))
	}
	payload := bytes.TrimSpace(line[len(dataTag):])
	return gjson.GetBytes(payload, "item_id").String()
}

func normalizedSSENestedItemID(t *testing.T, line []byte) string {
	t.Helper()
	if !bytes.HasPrefix(line, dataTag) {
		t.Fatalf("line does not start with data tag: %q", string(line))
	}
	payload := bytes.TrimSpace(line[len(dataTag):])
	return gjson.GetBytes(payload, "item.id").String()
}

func TestGitHubCopilotResponsesSSENormalizer_UsesOutputItemIDForReasoningEvents(t *testing.T) {
	t.Parallel()

	normalizer := newGitHubCopilotResponsesSSENormalizer()
	normalizedOutputItem := normalizer.NormalizeLine([]byte(`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"reasoning-item-id","type":"reasoning"}}`))
	if len(normalizedOutputItem) != 1 {
		t.Fatalf("output_item.added events = %d, want 1", len(normalizedOutputItem))
	}

	added := normalizer.NormalizeLine([]byte(`data: {"type":"response.reasoning_summary_part.added","item_id":"upstream-added-id","output_index":0,"summary_index":0}`))
	if len(added) != 1 {
		t.Fatalf("added events = %d, want 1", len(added))
	}
	if got := normalizedSSEItemID(t, added[0]); got != "reasoning-item-id" {
		t.Fatalf("added item_id = %q, want reasoning-item-id", got)
	}

	delta := normalizer.NormalizeLine([]byte(`data: {"type":"response.reasoning_summary_text.delta","item_id":"upstream-delta-id","output_index":0,"summary_index":0,"delta":"x"}`))
	if len(delta) != 1 {
		t.Fatalf("delta events = %d, want 1", len(delta))
	}
	if got := normalizedSSEItemID(t, delta[0]); got != "reasoning-item-id" {
		t.Fatalf("delta item_id = %q, want reasoning-item-id", got)
	}

	done := normalizer.NormalizeLine([]byte(`data: {"type":"response.reasoning_summary_part.done","item_id":"upstream-done-id","output_index":0,"summary_index":0}`))
	if len(done) != 1 {
		t.Fatalf("done events = %d, want 1", len(done))
	}
	if got := normalizedSSEItemID(t, done[0]); got != "reasoning-item-id" {
		t.Fatalf("done item_id = %q, want reasoning-item-id", got)
	}
}

func TestGitHubCopilotResponsesSSENormalizer_UsesOutputItemIDForContentEvents(t *testing.T) {
	t.Parallel()

	normalizer := newGitHubCopilotResponsesSSENormalizer()
	normalizedOutputItem := normalizer.NormalizeLine([]byte(`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"message-item-id","type":"message"}}`))
	if len(normalizedOutputItem) != 1 {
		t.Fatalf("output_item.added events = %d, want 1", len(normalizedOutputItem))
	}

	added := normalizer.NormalizeLine([]byte(`data: {"type":"response.content_part.added","item_id":"content-added","output_index":1,"content_index":0}`))
	if len(added) != 1 {
		t.Fatalf("content added events = %d, want 1", len(added))
	}
	if got := normalizedSSEItemID(t, added[0]); got != "message-item-id" {
		t.Fatalf("content added item_id = %q, want message-item-id", got)
	}

	delta := normalizer.NormalizeLine([]byte(`data: {"type":"response.output_text.delta","item_id":"content-delta","output_index":1,"content_index":0,"delta":"hello"}`))
	if len(delta) != 1 {
		t.Fatalf("output_text delta events = %d, want 1", len(delta))
	}
	if got := normalizedSSEItemID(t, delta[0]); got != "message-item-id" {
		t.Fatalf("output_text delta item_id = %q, want message-item-id", got)
	}

	other := normalizer.NormalizeLine([]byte(`data: {"type":"response.output_text.delta","item_id":"other-content","output_index":2,"content_index":0,"delta":"world"}`))
	if len(other) != 0 {
		t.Fatalf("expected unresolved output_index event to be buffered, got %d events", len(other))
	}
}

func TestGitHubCopilotResponsesSSENormalizer_BuffersUntilOutputItemAdded(t *testing.T) {
	t.Parallel()

	normalizer := newGitHubCopilotResponsesSSENormalizer()

	buffered := normalizer.NormalizeLine([]byte(`data: {"type":"response.output_text.delta","item_id":"upstream-message-id","output_index":3,"content_index":0,"delta":"Hello"}`))
	if len(buffered) != 0 {
		t.Fatalf("buffered unresolved event count = %d, want 0", len(buffered))
	}

	flushed := normalizer.NormalizeLine([]byte(`data: {"type":"response.output_item.added","output_index":3,"item":{"id":"real-message-id","type":"message"}}`))
	if len(flushed) != 2 {
		t.Fatalf("flushed events = %d, want 2 (output_item.added + buffered delta)", len(flushed))
	}
	if got := normalizedSSEItemID(t, flushed[1]); got != "real-message-id" {
		t.Fatalf("flushed buffered delta item_id = %q, want real-message-id", got)
	}
}

func TestGitHubCopilotResponsesSSENormalizer_UsesAddedIDWhenDoneIDDiffers(t *testing.T) {
	t.Parallel()

	normalizer := newGitHubCopilotResponsesSSENormalizer()
	normalizer.NormalizeLine([]byte(`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"canonical-added-id","type":"reasoning"}}`))

	normalizedDone := normalizer.NormalizeLine([]byte(`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"mismatched-done-id","type":"reasoning"}}`))
	if len(normalizedDone) != 1 {
		t.Fatalf("output_item.done events = %d, want 1", len(normalizedDone))
	}
	if got := normalizedSSENestedItemID(t, normalizedDone[0]); got != "canonical-added-id" {
		t.Fatalf("output_item.done item.id = %q, want canonical-added-id", got)
	}

	lateSummary := normalizer.NormalizeLine([]byte(`data: {"type":"response.reasoning_summary_part.added","item_id":"late-upstream-id","output_index":0,"summary_index":1}`))
	if len(lateSummary) != 1 {
		t.Fatalf("late reasoning summary events = %d, want 1", len(lateSummary))
	}
	if got := normalizedSSEItemID(t, lateSummary[0]); got != "canonical-added-id" {
		t.Fatalf("late reasoning summary item_id = %q, want canonical-added-id", got)
	}
}

func TestGitHubCopilotResponsesSSENormalizer_SynthesizesAddedWhenOnlyDoneSeen(t *testing.T) {
	t.Parallel()

	normalizer := newGitHubCopilotResponsesSSENormalizer()

	buffered := normalizer.NormalizeLine([]byte(`data: {"type":"response.reasoning_summary_part.added","item_id":"upstream-id","output_index":7,"summary_index":1}`))
	if len(buffered) != 0 {
		t.Fatalf("buffered unresolved summary count = %d, want 0", len(buffered))
	}

	flushed := normalizer.NormalizeLine([]byte(`data: {"type":"response.output_item.done","output_index":7,"item":{"id":"done-only-id","type":"reasoning"}}`))
	if len(flushed) != 3 {
		t.Fatalf("flushed events = %d, want 3 (synthetic added + buffered summary + done)", len(flushed))
	}
	if got := gjson.GetBytes(bytes.TrimSpace(flushed[0][len(dataTag):]), "type").String(); got != "response.output_item.added" {
		t.Fatalf("first flushed event type = %q, want response.output_item.added", got)
	}
	if got := normalizedSSEItemID(t, flushed[1]); got != "done-only-id" {
		t.Fatalf("flushed buffered summary item_id = %q, want done-only-id", got)
	}
	if got := gjson.GetBytes(bytes.TrimSpace(flushed[2][len(dataTag):]), "type").String(); got != "response.output_item.done" {
		t.Fatalf("last flushed event type = %q, want response.output_item.done", got)
	}
}

func TestGitHubCopilotResponsesSSENormalizer_NonDataLineUnchanged(t *testing.T) {
	t.Parallel()

	normalizer := newGitHubCopilotResponsesSSENormalizer()
	line := []byte("event: response.output_text.delta")
	normalized := normalizer.NormalizeLine(line)
	if len(normalized) != 1 {
		t.Fatalf("non-data line normalized count = %d, want 1", len(normalized))
	}
	if string(normalized[0]) != string(line) {
		t.Fatalf("non-data line changed: got %q want %q", string(normalized[0]), string(line))
	}
}

func TestGitHubCopilotNormalizeModel_StripsSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		model     string
		wantModel string
	}{
		{
			name:      "suffix stripped",
			model:     "claude-opus-4.6(medium)",
			wantModel: "claude-opus-4.6",
		},
		{
			name:      "no suffix unchanged",
			model:     "claude-opus-4.6",
			wantModel: "claude-opus-4.6",
		},
		{
			name:      "different suffix stripped",
			model:     "gpt-4o(high)",
			wantModel: "gpt-4o",
		},
		{
			name:      "numeric suffix stripped",
			model:     "gemini-2.5-pro(8192)",
			wantModel: "gemini-2.5-pro",
		},
	}

	e := &GitHubCopilotExecutor{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := []byte(`{"model":"` + tt.model + `","messages":[]}`)
			got := e.normalizeModel(tt.model, body)

			gotModel := gjson.GetBytes(got, "model").String()
			if gotModel != tt.wantModel {
				t.Fatalf("normalizeModel() model = %q, want %q", gotModel, tt.wantModel)
			}
		})
	}
}

func TestUseGitHubCopilotResponsesEndpoint_OpenAIResponseSource(t *testing.T) {
	t.Parallel()
	if !useGitHubCopilotResponsesEndpoint(sdktranslator.FromString("openai-response"), "claude-3-5-sonnet") {
		t.Fatal("expected openai-response source to use /responses")
	}
}

func TestUseGitHubCopilotResponsesEndpoint_CodexModel(t *testing.T) {
	t.Parallel()
	if !useGitHubCopilotResponsesEndpoint(sdktranslator.FromString("openai"), "gpt-5-codex") {
		t.Fatal("expected codex model to use /responses")
	}
}

func TestUseGitHubCopilotResponsesEndpoint_DefaultChat(t *testing.T) {
	t.Parallel()
	if useGitHubCopilotResponsesEndpoint(sdktranslator.FromString("openai"), "claude-3-5-sonnet") {
		t.Fatal("expected default openai source with non-codex model to use /chat/completions")
	}
}

func TestNormalizeGitHubCopilotChatTools_KeepFunctionOnly(t *testing.T) {
	t.Parallel()
	body := []byte(`{"tools":[{"type":"function","function":{"name":"ok"}},{"type":"code_interpreter"}],"tool_choice":"auto"}`)
	got := normalizeGitHubCopilotChatTools(body)
	tools := gjson.GetBytes(got, "tools").Array()
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	if tools[0].Get("type").String() != "function" {
		t.Fatalf("tool type = %q, want function", tools[0].Get("type").String())
	}
}

func TestNormalizeGitHubCopilotChatTools_InvalidToolChoiceDowngradeToAuto(t *testing.T) {
	t.Parallel()
	body := []byte(`{"tools":[],"tool_choice":{"type":"function","function":{"name":"x"}}}`)
	got := normalizeGitHubCopilotChatTools(body)
	if gjson.GetBytes(got, "tool_choice").String() != "auto" {
		t.Fatalf("tool_choice = %s, want auto", gjson.GetBytes(got, "tool_choice").Raw)
	}
}

func TestNormalizeGitHubCopilotResponsesInput_MissingInputExtractedFromSystemAndMessages(t *testing.T) {
	t.Parallel()
	body := []byte(`{"system":"sys text","messages":[{"role":"user","content":"user text"},{"role":"assistant","content":[{"type":"text","text":"assistant text"}]}]}`)
	got := normalizeGitHubCopilotResponsesInput(body)
	in := gjson.GetBytes(got, "input")
	if !in.IsArray() {
		t.Fatalf("input type = %v, want array", in.Type)
	}
	raw := in.Raw
	if !strings.Contains(raw, "sys text") || !strings.Contains(raw, "user text") || !strings.Contains(raw, "assistant text") {
		t.Fatalf("input = %s, want structured array with all texts", raw)
	}
	if gjson.GetBytes(got, "messages").Exists() {
		t.Fatal("messages should be removed after conversion")
	}
	if gjson.GetBytes(got, "system").Exists() {
		t.Fatal("system should be removed after conversion")
	}
}

func TestNormalizeGitHubCopilotResponsesInput_NonStringInputStringified(t *testing.T) {
	t.Parallel()
	body := []byte(`{"input":{"foo":"bar"}}`)
	got := normalizeGitHubCopilotResponsesInput(body)
	in := gjson.GetBytes(got, "input")
	if in.Type != gjson.String {
		t.Fatalf("input type = %v, want string", in.Type)
	}
	if !strings.Contains(in.String(), "foo") {
		t.Fatalf("input = %q, want stringified object", in.String())
	}
}

func TestNormalizeGitHubCopilotResponsesTools_FlattenFunctionTools(t *testing.T) {
	t.Parallel()
	body := []byte(`{"tools":[{"type":"function","function":{"name":"sum","description":"d","parameters":{"type":"object"}}},{"type":"web_search"}]}`)
	got := normalizeGitHubCopilotResponsesTools(body)
	tools := gjson.GetBytes(got, "tools").Array()
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	if tools[0].Get("name").String() != "sum" {
		t.Fatalf("tools[0].name = %q, want sum", tools[0].Get("name").String())
	}
	if !tools[0].Get("parameters").Exists() {
		t.Fatal("expected parameters to be preserved")
	}
}

func TestNormalizeGitHubCopilotResponsesTools_ClaudeFormatTools(t *testing.T) {
	t.Parallel()
	body := []byte(`{"tools":[{"name":"Bash","description":"Run commands","input_schema":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}},{"name":"Read","description":"Read files","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}]}`)
	got := normalizeGitHubCopilotResponsesTools(body)
	tools := gjson.GetBytes(got, "tools").Array()
	if len(tools) != 2 {
		t.Fatalf("tools len = %d, want 2", len(tools))
	}
	if tools[0].Get("type").String() != "function" {
		t.Fatalf("tools[0].type = %q, want function", tools[0].Get("type").String())
	}
	if tools[0].Get("name").String() != "Bash" {
		t.Fatalf("tools[0].name = %q, want Bash", tools[0].Get("name").String())
	}
	if tools[0].Get("description").String() != "Run commands" {
		t.Fatalf("tools[0].description = %q, want 'Run commands'", tools[0].Get("description").String())
	}
	if !tools[0].Get("parameters").Exists() {
		t.Fatal("expected parameters to be set from input_schema")
	}
	if tools[0].Get("parameters.properties.command").Exists() != true {
		t.Fatal("expected parameters.properties.command to exist")
	}
	if tools[1].Get("name").String() != "Read" {
		t.Fatalf("tools[1].name = %q, want Read", tools[1].Get("name").String())
	}
}

func TestNormalizeGitHubCopilotResponsesTools_FlattenToolChoiceFunctionObject(t *testing.T) {
	t.Parallel()
	body := []byte(`{"tool_choice":{"type":"function","function":{"name":"sum"}}}`)
	got := normalizeGitHubCopilotResponsesTools(body)
	if gjson.GetBytes(got, "tool_choice.type").String() != "function" {
		t.Fatalf("tool_choice.type = %q, want function", gjson.GetBytes(got, "tool_choice.type").String())
	}
	if gjson.GetBytes(got, "tool_choice.name").String() != "sum" {
		t.Fatalf("tool_choice.name = %q, want sum", gjson.GetBytes(got, "tool_choice.name").String())
	}
}

func TestNormalizeGitHubCopilotResponsesTools_InvalidToolChoiceDowngradeToAuto(t *testing.T) {
	t.Parallel()
	body := []byte(`{"tool_choice":{"type":"function"}}`)
	got := normalizeGitHubCopilotResponsesTools(body)
	if gjson.GetBytes(got, "tool_choice").String() != "auto" {
		t.Fatalf("tool_choice = %s, want auto", gjson.GetBytes(got, "tool_choice").Raw)
	}
}

func TestTranslateGitHubCopilotResponsesNonStreamToClaude_TextMapping(t *testing.T) {
	t.Parallel()
	resp := []byte(`{"id":"resp_1","model":"gpt-5-codex","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":3,"output_tokens":5}}`)
	out := translateGitHubCopilotResponsesNonStreamToClaude(resp)
	if gjson.Get(out, "type").String() != "message" {
		t.Fatalf("type = %q, want message", gjson.Get(out, "type").String())
	}
	if gjson.Get(out, "content.0.type").String() != "text" {
		t.Fatalf("content.0.type = %q, want text", gjson.Get(out, "content.0.type").String())
	}
	if gjson.Get(out, "content.0.text").String() != "hello" {
		t.Fatalf("content.0.text = %q, want hello", gjson.Get(out, "content.0.text").String())
	}
}

func TestTranslateGitHubCopilotResponsesNonStreamToClaude_ToolUseMapping(t *testing.T) {
	t.Parallel()
	resp := []byte(`{"id":"resp_2","model":"gpt-5-codex","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"sum","arguments":"{\"a\":1}"}],"usage":{"input_tokens":1,"output_tokens":2}}`)
	out := translateGitHubCopilotResponsesNonStreamToClaude(resp)
	if gjson.Get(out, "content.0.type").String() != "tool_use" {
		t.Fatalf("content.0.type = %q, want tool_use", gjson.Get(out, "content.0.type").String())
	}
	if gjson.Get(out, "content.0.name").String() != "sum" {
		t.Fatalf("content.0.name = %q, want sum", gjson.Get(out, "content.0.name").String())
	}
	if gjson.Get(out, "stop_reason").String() != "tool_use" {
		t.Fatalf("stop_reason = %q, want tool_use", gjson.Get(out, "stop_reason").String())
	}
}

func TestTranslateGitHubCopilotResponsesStreamToClaude_TextLifecycle(t *testing.T) {
	t.Parallel()
	var param any

	created := translateGitHubCopilotResponsesStreamToClaude([]byte(`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5-codex"}}`), &param)
	if len(created) == 0 || !strings.Contains(created[0], "message_start") {
		t.Fatalf("created events = %#v, want message_start", created)
	}

	delta := translateGitHubCopilotResponsesStreamToClaude([]byte(`data: {"type":"response.output_text.delta","delta":"he"}`), &param)
	joinedDelta := strings.Join(delta, "")
	if !strings.Contains(joinedDelta, "content_block_start") || !strings.Contains(joinedDelta, "text_delta") {
		t.Fatalf("delta events = %#v, want content_block_start + text_delta", delta)
	}

	completed := translateGitHubCopilotResponsesStreamToClaude([]byte(`data: {"type":"response.completed","response":{"usage":{"input_tokens":7,"output_tokens":9}}}`), &param)
	joinedCompleted := strings.Join(completed, "")
	if !strings.Contains(joinedCompleted, "message_delta") || !strings.Contains(joinedCompleted, "message_stop") {
		t.Fatalf("completed events = %#v, want message_delta + message_stop", completed)
	}
}

// --- Tests for X-Initiator detection logic (Problem L) ---

func TestApplyHeaders_XInitiator_UserOnly(t *testing.T) {
	t.Parallel()
	e := &GitHubCopilotExecutor{}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	body := []byte(`{"messages":[{"role":"system","content":"sys"},{"role":"user","content":"hello"}]}`)
	e.applyHeaders(req, "token", body)
	if got := req.Header.Get("X-Initiator"); got != "user" {
		t.Fatalf("X-Initiator = %q, want user", got)
	}
}

func TestApplyHeaders_XInitiator_AgentWithAssistantAndUserToolResult(t *testing.T) {
	t.Parallel()
	e := &GitHubCopilotExecutor{}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	// Claude Code typical flow: last message is user (tool result), but has assistant in history
	body := []byte(`{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"I will read the file"},{"role":"user","content":"tool result here"}]}`)
	e.applyHeaders(req, "token", body)
	if got := req.Header.Get("X-Initiator"); got != "agent" {
		t.Fatalf("X-Initiator = %q, want agent (assistant exists in messages)", got)
	}
}

func TestApplyHeaders_XInitiator_AgentWithToolRole(t *testing.T) {
	t.Parallel()
	e := &GitHubCopilotExecutor{}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	body := []byte(`{"messages":[{"role":"user","content":"hello"},{"role":"tool","content":"result"}]}`)
	e.applyHeaders(req, "token", body)
	if got := req.Header.Get("X-Initiator"); got != "agent" {
		t.Fatalf("X-Initiator = %q, want agent (tool role exists)", got)
	}
}

// --- Tests for x-github-api-version header (Problem M) ---

func TestApplyHeaders_GitHubAPIVersion(t *testing.T) {
	t.Parallel()
	e := &GitHubCopilotExecutor{}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	e.applyHeaders(req, "token", nil)
	if got := req.Header.Get("X-Github-Api-Version"); got != "2025-04-01" {
		t.Fatalf("X-Github-Api-Version = %q, want 2025-04-01", got)
	}
}

// --- Tests for vision detection (Problem P) ---

func TestDetectVisionContent_WithImageURL(t *testing.T) {
	t.Parallel()
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]}]}`)
	if !detectVisionContent(body) {
		t.Fatal("expected vision content to be detected")
	}
}

func TestDetectVisionContent_WithImageType(t *testing.T) {
	t.Parallel()
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"image","source":{"data":"abc","media_type":"image/png"}}]}]}`)
	if !detectVisionContent(body) {
		t.Fatal("expected image type to be detected")
	}
}

func TestDetectVisionContent_NoVision(t *testing.T) {
	t.Parallel()
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	if detectVisionContent(body) {
		t.Fatal("expected no vision content")
	}
}

func TestDetectVisionContent_NoMessages(t *testing.T) {
	t.Parallel()
	// After Responses API normalization, messages is removed — detection should return false
	body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
	if detectVisionContent(body) {
		t.Fatal("expected no vision content when messages field is absent")
	}
}
