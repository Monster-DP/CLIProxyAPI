package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	runtimeexecutor "github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func TestManagerExecuteStream_CodexErrorEventFallsBackToLowerPriorityAuth(t *testing.T) {
	model := "gpt-5.4"

	var (
		mu       sync.Mutex
		attempts []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		apiKey := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))

		mu.Lock()
		attempts = append(attempts, apiKey)
		mu.Unlock()

		switch apiKey {
		case "high-key":
			_, _ = w.Write([]byte("data: {\"type\":\"error\",\"status\":429,\"error\":{\"type\":\"server_error\",\"message\":\"Selected model is at capacity. Please try a different model.\"}}\n\n"))
		case "low-key":
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]},\"output_index\":0}\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_low\",\"object\":\"response\",\"created_at\":0,\"status\":\"completed\",\"background\":false,\"error\":null}}\n\n"))
		default:
			http.Error(w, "unexpected auth", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	manager := cliproxyauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(runtimeexecutor.NewCodexExecutor(&config.Config{}))

	highAuth := &cliproxyauth.Auth{
		ID:       "aa-high-auth",
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "high-key",
			"base_url": server.URL,
			"priority": "10",
		},
	}
	lowAuth := &cliproxyauth.Auth{
		ID:       "bb-low-auth",
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "low-key",
			"base_url": server.URL,
			"priority": "0",
		},
	}

	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(highAuth.ID, "codex", []*registry.ModelInfo{{ID: model}})
	reg.RegisterClient(lowAuth.ID, "codex", []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() {
		reg.UnregisterClient(highAuth.ID)
		reg.UnregisterClient(lowAuth.ID)
	})

	if _, err := manager.Register(context.Background(), highAuth); err != nil {
		t.Fatalf("register high auth: %v", err)
	}
	if _, err := manager.Register(context.Background(), lowAuth); err != nil {
		t.Fatalf("register low auth: %v", err)
	}

	result, err := manager.ExecuteStream(context.Background(), []string{"codex"}, cliproxyexecutor.Request{
		Model:   model,
		Payload: []byte(`{"model":"gpt-5.4","input":"hello"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error = %v, want fallback success", err)
	}

	var payload []byte
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v, want success via fallback auth", chunk.Err)
		}
		payload = append(payload, chunk.Payload...)
	}
	if !strings.Contains(string(payload), "\"resp_low\"") {
		t.Fatalf("payload = %q, want fallback response from low auth", string(payload))
	}

	mu.Lock()
	gotAttempts := append([]string(nil), attempts...)
	mu.Unlock()
	wantAttempts := []string{"high-key", "low-key"}
	if len(gotAttempts) != len(wantAttempts) {
		t.Fatalf("attempts = %v, want %v", gotAttempts, wantAttempts)
	}
	for i := range wantAttempts {
		if gotAttempts[i] != wantAttempts[i] {
			t.Fatalf("attempt %d = %q, want %q", i, gotAttempts[i], wantAttempts[i])
		}
	}
}
