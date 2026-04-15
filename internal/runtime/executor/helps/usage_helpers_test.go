package helps

import (
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestParseOpenAIUsageChatCompletions(t *testing.T) {
	data := []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":5}}}`)
	detail := ParseOpenAIUsage(data)
	if detail.InputTokens != 1 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 1)
	}
	if detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 2)
	}
	if detail.TotalTokens != 3 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 3)
	}
	if detail.CachedTokens != 4 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 4)
	}
	if detail.ReasoningTokens != 5 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 5)
	}
}

func TestParseOpenAIUsageResponses(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"cached_tokens":7},"output_tokens_details":{"reasoning_tokens":9}}}`)
	detail := ParseOpenAIUsage(data)
	if detail.InputTokens != 10 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 10)
	}
	if detail.OutputTokens != 20 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 20)
	}
	if detail.TotalTokens != 30 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 30)
	}
	if detail.CachedTokens != 7 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 7)
	}
	if detail.ReasoningTokens != 9 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 9)
	}
}

func TestUsageReporterBuildRecordIncludesLatency(t *testing.T) {
	reporter := &UsageReporter{
		provider:    "openai",
		model:       "gpt-5.4",
		requestedAt: time.Now().Add(-1500 * time.Millisecond),
	}

	record := reporter.buildRecord(usage.Detail{TotalTokens: 3}, false)
	if record.Latency < time.Second {
		t.Fatalf("latency = %v, want >= 1s", record.Latency)
	}
	if record.Latency > 3*time.Second {
		t.Fatalf("latency = %v, want <= 3s", record.Latency)
	}
}

func TestUsageReporterBuildRecordIncludesStreamingTimings(t *testing.T) {
	base := time.Date(2026, 4, 14, 20, 0, 0, 0, time.UTC)
	reporter := &UsageReporter{
		provider:      "codex",
		model:         "gpt-5.4",
		requestedAt:   base,
		firstOutputAt: base.Add(1200 * time.Millisecond),
		now: func() time.Time {
			return base.Add(4700 * time.Millisecond)
		},
	}

	record := reporter.buildRecord(usage.Detail{OutputTokens: 70, TotalTokens: 70}, false)
	if record.FirstTokenLatency != 1200*time.Millisecond {
		t.Fatalf("first token latency = %v, want %v", record.FirstTokenLatency, 1200*time.Millisecond)
	}
	if record.GenerationDuration != 3500*time.Millisecond {
		t.Fatalf("generation duration = %v, want %v", record.GenerationDuration, 3500*time.Millisecond)
	}
}

func TestUsageReporterBuildRecordUsesUnknownTimingSentinelWhenStreamingStartIsMissing(t *testing.T) {
	base := time.Date(2026, 4, 14, 20, 0, 0, 0, time.UTC)
	reporter := &UsageReporter{
		provider:    "codex",
		model:       "gpt-5.4",
		requestedAt: base,
		now: func() time.Time {
			return base.Add(4700 * time.Millisecond)
		},
	}

	record := reporter.buildRecord(usage.Detail{OutputTokens: 70, TotalTokens: 70}, false)
	if record.FirstTokenLatency >= 0 {
		t.Fatalf("first token latency = %v, want negative unknown sentinel", record.FirstTokenLatency)
	}
	if record.GenerationDuration >= 0 {
		t.Fatalf("generation duration = %v, want negative unknown sentinel", record.GenerationDuration)
	}
}

func TestUsageReporterBuildRecordUsesNegativeSentinelForUnknownStreamingTimings(t *testing.T) {
	base := time.Date(2026, 4, 14, 20, 0, 0, 0, time.UTC)
	reporter := &UsageReporter{
		provider:    "codex",
		model:       "gpt-5.4",
		requestedAt: base,
		now: func() time.Time {
			return base.Add(4700 * time.Millisecond)
		},
	}

	record := reporter.buildRecord(usage.Detail{OutputTokens: 70, TotalTokens: 70}, false)
	if record.FirstTokenLatency >= 0 {
		t.Fatalf("first token latency = %v, want negative sentinel for unknown timing", record.FirstTokenLatency)
	}
	if record.GenerationDuration >= 0 {
		t.Fatalf("generation duration = %v, want negative sentinel for unknown timing", record.GenerationDuration)
	}
}

func TestCodexStreamEventHasVisibleOutput(t *testing.T) {
	if !CodexStreamEventHasVisibleOutput([]byte(`{"type":"response.output_text.delta","delta":"hello"}`)) {
		t.Fatal("expected codex delta event to count as visible output")
	}
	if CodexStreamEventHasVisibleOutput([]byte(`{"type":"response.completed","response":{"status":"completed"}}`)) {
		t.Fatal("response.completed should not count as visible output")
	}
	if CodexStreamEventHasVisibleOutput([]byte(`{"type":"response.output_item.done","item":{"role":"assistant","content":[{"type":"tool_call","name":"lookup"}]}}`)) {
		t.Fatal("tool call content should not count as visible output")
	}
}

func TestOpenAIStreamEventHasVisibleOutput(t *testing.T) {
	if !OpenAIStreamEventHasVisibleOutput([]byte(`data: {"choices":[{"delta":{"content":"hello"}}]}`)) {
		t.Fatal("expected chat delta content to count as visible output")
	}
	if OpenAIStreamEventHasVisibleOutput([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":3}}`)) {
		t.Fatal("usage-only chunk should not count as visible output")
	}
}

func TestClaudeStreamEventHasVisibleOutput(t *testing.T) {
	if !ClaudeStreamEventHasVisibleOutput([]byte(`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`)) {
		t.Fatal("expected claude text delta to count as visible output")
	}
	if ClaudeStreamEventHasVisibleOutput([]byte(`data: {"type":"message_delta","usage":{"output_tokens":12}}`)) {
		t.Fatal("claude usage chunk should not count as visible output")
	}
}

func TestGeminiStreamPayloadHasVisibleOutput(t *testing.T) {
	if !GeminiStreamPayloadHasVisibleOutput([]byte(`data: {"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`)) {
		t.Fatal("expected gemini text part to count as visible output")
	}
	if !GeminiStreamPayloadHasVisibleOutput([]byte(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}}`)) {
		t.Fatal("expected gemini-cli response text part to count as visible output")
	}
	if GeminiStreamPayloadHasVisibleOutput([]byte(`data: {"usageMetadata":{"totalTokenCount":12}}`)) {
		t.Fatal("usage-only gemini chunk should not count as visible output")
	}
	if GeminiStreamPayloadHasVisibleOutput([]byte(`data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup_weather"}}]}}]}`)) {
		t.Fatal("function call part should not count as visible output")
	}
}
