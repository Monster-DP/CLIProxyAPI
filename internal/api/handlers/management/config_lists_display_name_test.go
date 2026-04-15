package management

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func newJSONPatchContext(t *testing.T, body string) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/provider", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	return rec, ctx
}

func TestPatchProviderConfig_UpdatesDisplayName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		handler    func(*Handler, *gin.Context)
		getDisplay func(*config.Config) string
		cfg        *config.Config
	}{
		{
			name:    "gemini",
			handler: (*Handler).PatchGeminiKey,
			cfg: &config.Config{
				GeminiKey: []config.GeminiKey{{APIKey: "gem-key", BaseURL: "https://api.asxs.top/v1"}},
			},
			getDisplay: func(cfg *config.Config) string { return cfg.GeminiKey[0].DisplayName },
		},
		{
			name:    "codex",
			handler: (*Handler).PatchCodexKey,
			cfg: &config.Config{
				CodexKey: []config.CodexKey{{APIKey: "codex-key", BaseURL: "https://api.openrouter.ai/v1"}},
			},
			getDisplay: func(cfg *config.Config) string { return cfg.CodexKey[0].DisplayName },
		},
		{
			name:    "claude",
			handler: (*Handler).PatchClaudeKey,
			cfg: &config.Config{
				ClaudeKey: []config.ClaudeKey{{APIKey: "claude-key", BaseURL: "https://api.anthropic.com"}},
			},
			getDisplay: func(cfg *config.Config) string { return cfg.ClaudeKey[0].DisplayName },
		},
		{
			name:    "vertex",
			handler: (*Handler).PatchVertexCompatKey,
			cfg: &config.Config{
				VertexCompatAPIKey: []config.VertexCompatKey{{APIKey: "vertex-key", BaseURL: "https://api.vertex.example.com"}},
			},
			getDisplay: func(cfg *config.Config) string { return cfg.VertexCompatAPIKey[0].DisplayName },
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := &Handler{
				cfg:            tc.cfg,
				configFilePath: writeTestConfigFile(t),
			}

			rec, ctx := newJSONPatchContext(t, `{"index":0,"value":{"display-name":" Friendly Provider "}}`)

			tc.handler(h, ctx)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}

			if got := tc.getDisplay(h.cfg); got != "Friendly Provider" {
				t.Fatalf("display-name = %q, want %q", got, "Friendly Provider")
			}
		})
	}
}

func TestPutProviderConfig_PreservesDisplayName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		path       string
		body       string
		want       string
		handler    func(*Handler, *gin.Context)
		getDisplay func(*config.Config) string
	}{
		{
			name:    "gemini",
			path:    "/v0/management/gemini-api-key",
			body:    `[{"api-key":"gem-key","base-url":"https://api.asxs.top/v1","display-name":" ASXS "}]`,
			want:    "ASXS",
			handler: (*Handler).PutGeminiKeys,
			getDisplay: func(cfg *config.Config) string {
				if len(cfg.GeminiKey) == 0 {
					return ""
				}
				return cfg.GeminiKey[0].DisplayName
			},
		},
		{
			name:    "codex",
			path:    "/v0/management/codex-api-key",
			body:    `[{"api-key":"codex-key","base-url":"https://api.openrouter.ai/v1","display-name":" OpenRouter Codex "}]`,
			want:    "OpenRouter Codex",
			handler: (*Handler).PutCodexKeys,
			getDisplay: func(cfg *config.Config) string {
				if len(cfg.CodexKey) == 0 {
					return ""
				}
				return cfg.CodexKey[0].DisplayName
			},
		},
		{
			name:    "claude",
			path:    "/v0/management/claude-api-key",
			body:    `[{"api-key":"claude-key","base-url":"https://api.anthropic.com","display-name":" Anthropic Proxy "}]`,
			want:    "Anthropic Proxy",
			handler: (*Handler).PutClaudeKeys,
			getDisplay: func(cfg *config.Config) string {
				if len(cfg.ClaudeKey) == 0 {
					return ""
				}
				return cfg.ClaudeKey[0].DisplayName
			},
		},
		{
			name:    "vertex",
			path:    "/v0/management/vertex-api-key",
			body:    `[{"api-key":"vertex-key","base-url":"https://gateway.vertex.example.com","display-name":" Vertex Edge "}]`,
			want:    "Vertex Edge",
			handler: (*Handler).PutVertexCompatKeys,
			getDisplay: func(cfg *config.Config) string {
				if len(cfg.VertexCompatAPIKey) == 0 {
					return ""
				}
				return cfg.VertexCompatAPIKey[0].DisplayName
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := &Handler{
				cfg:            &config.Config{},
				configFilePath: writeTestConfigFile(t),
			}

			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			req := httptest.NewRequest(http.MethodPut, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			ctx.Request = req

			tc.handler(h, ctx)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if got := tc.getDisplay(h.cfg); got != tc.want {
				t.Fatalf("display-name = %q, want %q", got, tc.want)
			}
		})
	}
}
