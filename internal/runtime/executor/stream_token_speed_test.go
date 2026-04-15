package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

type usageRecordProbe struct {
	provider string
	authID   string
	records  chan coreusage.Record
}

func (p *usageRecordProbe) HandleUsage(_ context.Context, record coreusage.Record) {
	if p == nil {
		return
	}
	if record.Provider != p.provider || record.AuthID != p.authID {
		return
	}
	select {
	case p.records <- record:
	default:
	}
}

func TestCodexExecutorExecuteStream_UsageRecordCapturesVisibleOutputTimings(t *testing.T) {
	probe := &usageRecordProbe{
		provider: "codex",
		authID:   "auth-token-speed-codex",
		records:  make(chan coreusage.Record, 1),
	}
	coreusage.DefaultManager().Register(probe)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		time.Sleep(25 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		time.Sleep(35 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "auth-token-speed-codex",
		Attributes: map[string]string{
			"base_url": server.URL,
			"api_key":  "test",
		},
	}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","input":"hello"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error = %v, want nil", err)
	}

	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v, want nil", chunk.Err)
		}
	}

	record := waitForUsageRecord(t, probe.records)
	if record.FirstTokenLatency <= 0 {
		t.Fatalf("FirstTokenLatency = %v, want > 0", record.FirstTokenLatency)
	}
	if record.GenerationDuration <= 0 {
		t.Fatalf("GenerationDuration = %v, want > 0", record.GenerationDuration)
	}
	if record.Detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", record.Detail.OutputTokens, 2)
	}
}

func TestCodexExecutorExecuteStream_UsageRecordUsesFirstChunkTimingForTTFB(t *testing.T) {
	probe := &usageRecordProbe{
		provider: "codex",
		authID:   "auth-token-speed-codex-ttfb",
		records:  make(chan coreusage.Record, 1),
	}
	coreusage.DefaultManager().Register(probe)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\",\"status\":\"in_progress\"}}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		time.Sleep(90 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "auth-token-speed-codex-ttfb",
		Attributes: map[string]string{
			"base_url": server.URL,
			"api_key":  "test",
		},
	}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","input":"hello"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error = %v, want nil", err)
	}

	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v, want nil", chunk.Err)
		}
	}

	record := waitForUsageRecord(t, probe.records)
	if record.FirstTokenLatency <= 0 {
		t.Fatalf("FirstTokenLatency = %v, want > 0", record.FirstTokenLatency)
	}
	if record.FirstTokenLatency >= 70*time.Millisecond {
		t.Fatalf("FirstTokenLatency = %v, want < 70ms so TTFB uses first response chunk instead of first visible text", record.FirstTokenLatency)
	}
	if record.GenerationDuration <= 90*time.Millisecond {
		t.Fatalf("GenerationDuration = %v, want > 90ms so decode duration starts at first response chunk", record.GenerationDuration)
	}
}

func TestOpenAICompatExecutorExecuteStream_UsageRecordCapturesVisibleOutputTimings(t *testing.T) {
	probe := &usageRecordProbe{
		provider: "openrouter",
		authID:   "auth-token-speed-openai",
		records:  make(chan coreusage.Record, 1),
	}
	coreusage.DefaultManager().Register(probe)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openrouter", &config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "auth-token-speed-openai",
		Attributes: map[string]string{
			"base_url": server.URL,
			"api_key":  "test",
		},
	}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4o-mini",
		Payload: []byte(`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error = %v, want nil", err)
	}

	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v, want nil", chunk.Err)
		}
	}

	record := waitForUsageRecord(t, probe.records)
	if record.FirstTokenLatency <= 0 {
		t.Fatalf("FirstTokenLatency = %v, want > 0", record.FirstTokenLatency)
	}
	if record.GenerationDuration <= 0 {
		t.Fatalf("GenerationDuration = %v, want > 0", record.GenerationDuration)
	}
	if record.Detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", record.Detail.OutputTokens, 2)
	}
}

func TestOpenAICompatExecutorExecuteStream_IgnoresHeartbeatPreludeWhenRecordingTTFB(t *testing.T) {
	probe := &usageRecordProbe{
		provider: "openrouter",
		authID:   "auth-token-speed-openai-heartbeat",
		records:  make(chan coreusage.Record, 1),
	}
	coreusage.DefaultManager().Register(probe)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte(": ping\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		time.Sleep(90 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openrouter", &config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "auth-token-speed-openai-heartbeat",
		Attributes: map[string]string{
			"base_url": server.URL,
			"api_key":  "test",
		},
	}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-4o-mini",
		Payload: []byte(`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error = %v, want nil", err)
	}

	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v, want nil", chunk.Err)
		}
	}

	record := waitForUsageRecord(t, probe.records)
	if record.FirstTokenLatency <= 70*time.Millisecond {
		t.Fatalf("FirstTokenLatency = %v, want > 70ms so heartbeat/comment prelude does not become TTFB", record.FirstTokenLatency)
	}
}

func TestGeminiVertexExecutorExecuteStreamWithAPIKey_UsageRecordCapturesVisibleOutputTimings(t *testing.T) {
	probe := &usageRecordProbe{
		provider: "vertex",
		authID:   "auth-token-speed-vertex-api-key",
		records:  make(chan coreusage.Record, 1),
	}
	coreusage.DefaultManager().Register(probe)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/publishers/google/models/gemini-2.5-pro:streamGenerateContent" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.RawQuery != "alt=sse" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Header.Get("x-goog-api-key") != "test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2,\"totalTokenCount\":3}}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	executor := NewGeminiVertexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "auth-token-speed-vertex-api-key",
		Attributes: map[string]string{
			"base_url": server.URL,
			"api_key":  "test",
		},
	}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gemini-2.5-pro",
		Payload: []byte(`{"model":"gemini-2.5-pro","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("gemini"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error = %v, want nil", err)
	}

	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v, want nil", chunk.Err)
		}
	}

	record := waitForUsageRecord(t, probe.records)
	if record.FirstTokenLatency <= 0 {
		t.Fatalf("FirstTokenLatency = %v, want > 0", record.FirstTokenLatency)
	}
	if record.GenerationDuration <= 0 {
		t.Fatalf("GenerationDuration = %v, want > 0", record.GenerationDuration)
	}
	if record.Detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", record.Detail.OutputTokens, 2)
	}
}

func TestAntigravityExecutorExecuteStream_UsageRecordCapturesVisibleOutputTimings(t *testing.T) {
	probe := &usageRecordProbe{
		provider: "antigravity",
		authID:   "auth-token-speed-antigravity",
		records:  make(chan coreusage.Record, 1),
	}
	coreusage.DefaultManager().Register(probe)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:streamGenerateContent" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.RawQuery != "alt=sse" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-access-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"response\":{\"candidates\":[{\"finishReason\":\"STOP\",\"content\":{\"parts\":[]}}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2,\"totalTokenCount\":3}}}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	executor := NewAntigravityExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID: "auth-token-speed-antigravity",
		Attributes: map[string]string{
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"access_token": "test-access-token",
			"expired":      time.Now().Add(time.Hour).Format(time.RFC3339),
		},
	}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gemini-2.5-pro",
		Payload: []byte(`{"model":"gemini-2.5-pro","contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("gemini"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error = %v, want nil", err)
	}

	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v, want nil", chunk.Err)
		}
	}

	record := waitForUsageRecord(t, probe.records)
	if record.FirstTokenLatency <= 0 {
		t.Fatalf("FirstTokenLatency = %v, want > 0", record.FirstTokenLatency)
	}
	if record.GenerationDuration <= 0 {
		t.Fatalf("GenerationDuration = %v, want > 0", record.GenerationDuration)
	}
	if record.Detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", record.Detail.OutputTokens, 2)
	}
}

func waitForUsageRecord(t *testing.T, records <-chan coreusage.Record) coreusage.Record {
	t.Helper()

	select {
	case record := <-records:
		return record
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for usage record")
		return coreusage.Record{}
	}
}
