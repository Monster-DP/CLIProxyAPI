package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestGetUsageEventsPaginatesAndFilters(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	previousEnabled := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() {
		usage.SetStatisticsEnabled(previousEnabled)
	})

	stats := usage.NewRequestStatistics()
	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	baseTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	records := []coreusage.Record{
		{
			APIKey:                    "api-1",
			Model:                     "gpt-5.4",
			RequestedAt:               baseTime.Add(-3 * time.Hour),
			ProviderFirstTokenLatency: 600 * time.Millisecond,
			FirstTokenLatency:         800 * time.Millisecond,
			GenerationDuration:        2 * time.Second,
			Source:                    "provider-a",
			AuthIndex:                 "codex:0",
			Detail:                    coreusage.Detail{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
		},
		{
			APIKey:                    "api-1",
			Model:                     "gpt-5.4",
			RequestedAt:               baseTime.Add(-2 * time.Hour),
			ProviderFirstTokenLatency: 550 * time.Millisecond,
			FirstTokenLatency:         780 * time.Millisecond,
			GenerationDuration:        2500 * time.Millisecond,
			Source:                    "provider-a",
			AuthIndex:                 "codex:1",
			Detail:                    coreusage.Detail{InputTokens: 11, OutputTokens: 21, TotalTokens: 32},
		},
		{
			APIKey:      "api-1",
			Model:       "gpt-4.1",
			RequestedAt: baseTime.Add(-1 * time.Hour),
			Source:      "provider-b",
			AuthIndex:   "openai:0",
			Detail:      coreusage.Detail{InputTokens: 12, OutputTokens: 22, TotalTokens: 34},
		},
	}

	for _, record := range records {
		stats.Record(nil, record)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/events?range=all&model=gpt-5.4&limit=1", nil)

	h := &Handler{usageStats: stats}
	h.GetUsageEvents(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		Items []struct {
			APIKey      string `json:"api_key"`
			Model       string `json:"model"`
			Source      string `json:"source"`
			AuthIndex   string `json:"auth_index"`
			EventCursor string `json:"event_cursor"`
		} `json:"items"`
		HasMore       bool   `json:"has_more"`
		NextBefore    string `json:"next_before"`
		TotalMatching int    `json:"total_matching"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload.TotalMatching != 2 {
		t.Fatalf("total_matching = %d, want 2", payload.TotalMatching)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(payload.Items))
	}
	if payload.Items[0].APIKey != "api-1" {
		t.Fatalf("api_key = %q, want api-1", payload.Items[0].APIKey)
	}
	if payload.Items[0].AuthIndex != "codex:1" {
		t.Fatalf("auth_index = %q, want codex:1", payload.Items[0].AuthIndex)
	}
	if !payload.HasMore {
		t.Fatal("has_more = false, want true")
	}
	if payload.NextBefore == "" {
		t.Fatal("next_before is empty")
	}
}

func TestGetUsageEventSummaryReturnsSpeedLeaderboards(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	previousEnabled := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() {
		usage.SetStatisticsEnabled(previousEnabled)
	})

	stats := usage.NewRequestStatistics()
	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	baseTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	records := []coreusage.Record{
		{
			APIKey:                    "api-1",
			Model:                     "gpt-5.4",
			RequestedAt:               baseTime.Add(-2 * time.Hour),
			ProviderFirstTokenLatency: 500 * time.Millisecond,
			GenerationDuration:        2 * time.Second,
			Source:                    "provider-a",
			AuthIndex:                 "codex:0",
			Detail:                    coreusage.Detail{OutputTokens: 20, TotalTokens: 20},
		},
		{
			APIKey:                    "api-1",
			Model:                     "gpt-5.4",
			RequestedAt:               baseTime.Add(-90 * time.Minute),
			ProviderFirstTokenLatency: 700 * time.Millisecond,
			GenerationDuration:        4 * time.Second,
			Source:                    "provider-a",
			AuthIndex:                 "codex:1",
			Detail:                    coreusage.Detail{OutputTokens: 16, TotalTokens: 16},
		},
		{
			APIKey:                    "api-2",
			Model:                     "gpt-4.1",
			RequestedAt:               baseTime.Add(-1 * time.Hour),
			ProviderFirstTokenLatency: 300 * time.Millisecond,
			GenerationDuration:        1 * time.Second,
			Source:                    "provider-b",
			AuthIndex:                 "openai:0",
			Detail:                    coreusage.Detail{OutputTokens: 10, TotalTokens: 10},
		},
	}

	for _, record := range records {
		stats.Record(nil, record)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/events/summary?range=all", nil)

	h := &Handler{usageStats: stats}
	h.GetUsageEventSummary(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		SourceItems []struct {
			ID                      string  `json:"id"`
			AverageTTFTMs           float64 `json:"average_ttft_ms"`
			AveragePreciseOutputTps float64 `json:"average_precise_output_tps"`
			SampleCount             int     `json:"sample_count"`
		} `json:"source_items"`
		ModelItems []struct {
			ID            string  `json:"id"`
			AverageTTFTMs float64 `json:"average_ttft_ms"`
			SampleCount   int     `json:"sample_count"`
		} `json:"model_items"`
		TotalMatching int `json:"total_matching"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload.TotalMatching != 3 {
		t.Fatalf("total_matching = %d, want 3", payload.TotalMatching)
	}
	if len(payload.SourceItems) != 2 {
		t.Fatalf("source_items len = %d, want 2", len(payload.SourceItems))
	}
	if payload.SourceItems[0].ID != "provider-b" {
		t.Fatalf("first source id = %q, want provider-b", payload.SourceItems[0].ID)
	}
	if payload.SourceItems[0].AverageTTFTMs != 300 {
		t.Fatalf("first source average_ttft_ms = %v, want 300", payload.SourceItems[0].AverageTTFTMs)
	}
	if len(payload.ModelItems) != 2 {
		t.Fatalf("model_items len = %d, want 2", len(payload.ModelItems))
	}
}

func TestGetUsageEventOptionsReturnsDistinctValues(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	previousEnabled := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() {
		usage.SetStatisticsEnabled(previousEnabled)
	})

	stats := usage.NewRequestStatistics()
	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	baseTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	for i := range 505 {
		stats.Record(nil, coreusage.Record{
			APIKey:      "api-1",
			Model:       "gpt-5.4",
			RequestedAt: baseTime.Add(-time.Duration(i) * time.Minute),
			Source:      "provider-a",
			AuthIndex:   "codex:0",
			Detail:      coreusage.Detail{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
		})
	}
	stats.Record(nil, coreusage.Record{
		APIKey:      "api-2",
		Model:       "gpt-4.1",
		RequestedAt: baseTime.Add(-24 * time.Hour),
		Source:      "provider-b",
		AuthIndex:   "openai:1",
		Detail:      coreusage.Detail{InputTokens: 5, OutputTokens: 10, TotalTokens: 15},
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/events/options?range=all", nil)

	h := &Handler{usageStats: stats}
	h.GetUsageEventOptions(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		Sources       []string `json:"sources"`
		Models        []string `json:"models"`
		AuthIndexes   []string `json:"auth_indexes"`
		TotalMatching int      `json:"total_matching"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload.TotalMatching != 506 {
		t.Fatalf("total_matching = %d, want 506", payload.TotalMatching)
	}
	if len(payload.Sources) != 2 || payload.Sources[0] != "provider-a" || payload.Sources[1] != "provider-b" {
		t.Fatalf("sources = %#v, want [provider-a provider-b]", payload.Sources)
	}
	if len(payload.Models) != 2 || payload.Models[0] != "gpt-4.1" || payload.Models[1] != "gpt-5.4" {
		t.Fatalf("models = %#v, want [gpt-4.1 gpt-5.4]", payload.Models)
	}
	if len(payload.AuthIndexes) != 2 || payload.AuthIndexes[0] != "codex:0" || payload.AuthIndexes[1] != "openai:1" {
		t.Fatalf("auth_indexes = %#v, want [codex:0 openai:1]", payload.AuthIndexes)
	}
}
