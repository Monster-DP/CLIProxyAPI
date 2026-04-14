package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

func TestCodexExecutorExecute_StreamErrorEventReturnsStatusErr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"error\",\"status\":429,\"error\":{\"type\":\"server_error\",\"message\":\"Selected model is at capacity. Please try a different model.\"}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","input":"hello"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
	})
	if err == nil {
		t.Fatalf("expected Execute error")
	}
	if got := statusCodeFromExecutorError(err); got != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d (err=%v)", got, http.StatusTooManyRequests, err)
	}
	if !strings.Contains(err.Error(), "Selected model is at capacity") {
		t.Fatalf("error = %q, want capacity message", err.Error())
	}
}

func TestCodexExecutorExecuteStream_StreamErrorEventReturnsChunkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"error\",\"status\":429,\"error\":{\"type\":\"server_error\",\"message\":\"Selected model is at capacity. Please try a different model.\"}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","input":"hello"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error = %v, want nil stream result with chunk error", err)
	}

	chunk, ok := <-result.Chunks
	if !ok {
		t.Fatalf("expected first stream chunk")
	}
	if len(chunk.Payload) != 0 {
		t.Fatalf("first chunk payload = %q, want empty payload with error", string(chunk.Payload))
	}
	if chunk.Err == nil {
		t.Fatalf("expected first chunk error")
	}
	if got := statusCodeFromExecutorError(chunk.Err); got != http.StatusTooManyRequests {
		t.Fatalf("chunk status = %d, want %d (err=%v)", got, http.StatusTooManyRequests, chunk.Err)
	}
	if !strings.Contains(chunk.Err.Error(), "Selected model is at capacity") {
		t.Fatalf("chunk error = %q, want capacity message", chunk.Err.Error())
	}
}

func statusCodeFromExecutorError(err error) int {
	if err == nil {
		return 0
	}
	if se, ok := errors.AsType[cliproxyexecutor.StatusError](err); ok && se != nil {
		return se.StatusCode()
	}
	return 0
}
