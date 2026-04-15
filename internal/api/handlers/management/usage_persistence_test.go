package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func TestImportUsageStatisticsPersistsSnapshotImmediately(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := usage.NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"version": 1,
		"usage": usage.StatisticsSnapshot{
			APIs: map[string]usage.APISnapshot{
				"test-key": {
					Models: map[string]usage.ModelSnapshot{
						"gpt-5.4": {
							Details: []usage.RequestDetail{{
								Timestamp: time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
								Tokens: usage.TokenStats{
									InputTokens:  10,
									OutputTokens: 20,
									TotalTokens:  30,
								},
							}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/usage/import", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h := &Handler{usageStats: stats}
	h.ImportUsageStatistics(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	snapshot, err := usage.LoadSnapshotFromFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFromFile returned error: %v", err)
	}
	if snapshot.TotalRequests != 1 {
		t.Fatalf("total_requests = %d, want 1", snapshot.TotalRequests)
	}
}

func TestImportUsageStatisticsConvertsLegacyZeroStreamingTimingsToUnknownSentinel(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	stats := usage.NewRequestStatistics()

	body, err := json.Marshal(map[string]any{
		"version": 1,
		"usage": usage.StatisticsSnapshot{
			APIs: map[string]usage.APISnapshot{
				"test-key": {
					Models: map[string]usage.ModelSnapshot{
						"gpt-5.4": {
							Details: []usage.RequestDetail{{
								Timestamp:            time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
								LatencyMs:            1200,
								FirstTokenLatencyMs:  0,
								GenerationDurationMs: 0,
								Tokens: usage.TokenStats{
									InputTokens:  10,
									OutputTokens: 20,
									TotalTokens:  30,
								},
							}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/usage/import", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h := &Handler{usageStats: stats}
	h.ImportUsageStatistics(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	snapshot := stats.Snapshot()
	detail := snapshot.APIs["test-key"].Models["gpt-5.4"].Details[0]
	if detail.FirstTokenLatencyMs >= 0 {
		t.Fatalf("first_token_latency_ms = %d, want negative unknown sentinel", detail.FirstTokenLatencyMs)
	}
	if detail.GenerationDurationMs >= 0 {
		t.Fatalf("generation_duration_ms = %d, want negative unknown sentinel", detail.GenerationDurationMs)
	}
}

func TestExportUsageStatisticsUsesCurrentVersion(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/export", nil)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	h.ExportUsageStatistics(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload usageExportPayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if payload.Version != currentUsageExportVersion {
		t.Fatalf("version = %d, want %d", payload.Version, currentUsageExportVersion)
	}
}

func TestImportUsageStatisticsVersionTwoPreservesZeroStreamingTimings(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	stats := usage.NewRequestStatistics()

	body, err := json.Marshal(map[string]any{
		"version": currentUsageExportVersion,
		"usage": usage.StatisticsSnapshot{
			APIs: map[string]usage.APISnapshot{
				"test-key": {
					Models: map[string]usage.ModelSnapshot{
						"gpt-5.4": {
							Details: []usage.RequestDetail{{
								Timestamp:            time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
								LatencyMs:            1200,
								FirstTokenLatencyMs:  0,
								GenerationDurationMs: 0,
								Tokens: usage.TokenStats{
									InputTokens:  10,
									OutputTokens: 20,
									TotalTokens:  30,
								},
							}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/usage/import", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h := &Handler{usageStats: stats}
	h.ImportUsageStatistics(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	snapshot := stats.Snapshot()
	detail := snapshot.APIs["test-key"].Models["gpt-5.4"].Details[0]
	if detail.FirstTokenLatencyMs != 0 {
		t.Fatalf("first_token_latency_ms = %d, want 0", detail.FirstTokenLatencyMs)
	}
	if detail.GenerationDurationMs != 0 {
		t.Fatalf("generation_duration_ms = %d, want 0", detail.GenerationDurationMs)
	}
}
