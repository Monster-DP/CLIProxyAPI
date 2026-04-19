package management

import (
	"bytes"
	"context"
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

func TestExportUsageStatisticsIncludesArchivedDetailsWhenPersistenceEnabled(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := usage.NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}
	result := stats.MergeSnapshot(usage.StatisticsSnapshot{
		APIs: map[string]usage.APISnapshot{
			"test-key": {
				Models: map[string]usage.ModelSnapshot{
					"gpt-5.4": {
						Details: []usage.RequestDetail{{
							Timestamp: time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
							Source:    "provider-a",
							AuthIndex: "codex:0",
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
	})
	if result.Added != 1 {
		t.Fatalf("added = %d, want 1", result.Added)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/export", nil)

	h := &Handler{usageStats: stats}
	h.ExportUsageStatistics(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload usageExportPayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	details := payload.Usage.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].Source != "provider-a" {
		t.Fatalf("detail source = %q, want provider-a", details[0].Source)
	}
}

func TestGetUsageStatisticsIncludesArchivedDetailsWhenPersistenceEnabled(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := usage.NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	stats.Record(context.Background(), coreusage.Record{
		APIKey:                    "archive-key",
		Model:                     "gpt-5.4",
		RequestedAt:               time.Date(2026, 4, 17, 13, 0, 0, 0, time.UTC),
		Latency:                   1100 * time.Millisecond,
		ProviderFirstTokenLatency: 500 * time.Millisecond,
		FirstTokenLatency:         700 * time.Millisecond,
		GenerationDuration:        2100 * time.Millisecond,
		Source:                    "provider-a",
		AuthIndex:                 "codex:1",
		Detail: coreusage.Detail{
			InputTokens:  9,
			OutputTokens: 21,
			TotalTokens:  30,
		},
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage", nil)

	h := &Handler{usageStats: stats}
	h.GetUsageStatistics(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		Usage usage.StatisticsSnapshot `json:"usage"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	details := payload.Usage.APIs["archive-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].Source != "provider-a" {
		t.Fatalf("detail source = %q, want provider-a", details[0].Source)
	}
}

func TestImportUsageStatisticsRestoresArchivedHistoryFromExportedPersistenceBackedSnapshot(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	previousEnabled := usage.StatisticsEnabled()
	usage.SetStatisticsEnabled(true)
	t.Cleanup(func() {
		usage.SetStatisticsEnabled(previousEnabled)
	})

	sourceStats := usage.NewRequestStatistics()
	sourcePath := filepath.Join(t.TempDir(), "source", "usage-statistics.json")
	if err := sourceStats.EnablePersistence(sourcePath); err != nil {
		t.Fatalf("source EnablePersistence returned error: %v", err)
	}

	sourceStats.Record(context.Background(), coreusage.Record{
		APIKey:                    "archive-key",
		Model:                     "gpt-5.4",
		RequestedAt:               time.Date(2026, 4, 17, 13, 0, 0, 0, time.UTC),
		Latency:                   1100 * time.Millisecond,
		ProviderFirstTokenLatency: 500 * time.Millisecond,
		FirstTokenLatency:         700 * time.Millisecond,
		GenerationDuration:        2100 * time.Millisecond,
		Source:                    "provider-b",
		AuthIndex:                 "codex:1",
		Detail: coreusage.Detail{
			InputTokens:  9,
			OutputTokens: 21,
			TotalTokens:  30,
		},
	})

	exportRecorder := httptest.NewRecorder()
	exportContext, _ := gin.CreateTestContext(exportRecorder)
	exportContext.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/export", nil)

	sourceHandler := &Handler{usageStats: sourceStats}
	sourceHandler.ExportUsageStatistics(exportContext)

	if exportRecorder.Code != http.StatusOK {
		t.Fatalf("export status = %d, want %d, body=%s", exportRecorder.Code, http.StatusOK, exportRecorder.Body.String())
	}

	destStats := usage.NewRequestStatistics()
	destPath := filepath.Join(t.TempDir(), "dest", "usage-statistics.json")
	if err := destStats.EnablePersistence(destPath); err != nil {
		t.Fatalf("dest EnablePersistence returned error: %v", err)
	}

	importRecorder := httptest.NewRecorder()
	importContext, _ := gin.CreateTestContext(importRecorder)
	importContext.Request = httptest.NewRequest(
		http.MethodPost,
		"/v0/management/usage/import",
		bytes.NewReader(exportRecorder.Body.Bytes()),
	)
	importContext.Request.Header.Set("Content-Type", "application/json")

	destHandler := &Handler{usageStats: destStats}
	destHandler.ImportUsageStatistics(importContext)

	if importRecorder.Code != http.StatusOK {
		t.Fatalf("import status = %d, want %d, body=%s", importRecorder.Code, http.StatusOK, importRecorder.Body.String())
	}

	page, err := destStats.QueryEvents(usage.EventQuery{Limit: 10})
	if err != nil {
		t.Fatalf("QueryEvents returned error: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(page.Items))
	}
	if page.Items[0].Source != "provider-b" {
		t.Fatalf("source = %q, want provider-b", page.Items[0].Source)
	}
	if destStats.SummarySnapshot().TotalRequests != 1 {
		t.Fatalf("summary total_requests = %d, want 1", destStats.SummarySnapshot().TotalRequests)
	}
}

func TestImportUsageStatisticsSkipsDuplicateArchivedHistoryOnRepeatedImport(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	stats := usage.NewRequestStatistics()
	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"version": currentUsageExportVersion,
		"usage": usage.StatisticsSnapshot{
			APIs: map[string]usage.APISnapshot{
				"test-key": {
					Models: map[string]usage.ModelSnapshot{
						"gpt-5.4": {
							Details: []usage.RequestDetail{{
								Timestamp:                   time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
								LatencyMs:                   1200,
								ProviderFirstTokenLatencyMs: 300,
								FirstTokenLatencyMs:         450,
								GenerationDurationMs:        750,
								Source:                      "codex",
								AuthIndex:                   "codex:0",
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

	importUsagePayload := func() map[string]int64 {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v0/management/usage/import", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h := &Handler{usageStats: stats}
		h.ImportUsageStatistics(c)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
		}

		var payload map[string]int64
		if errUnmarshal := json.Unmarshal(recorder.Body.Bytes(), &payload); errUnmarshal != nil {
			t.Fatalf("json.Unmarshal returned error: %v", errUnmarshal)
		}
		return payload
	}

	first := importUsagePayload()
	if first["added"] != 1 || first["skipped"] != 0 {
		t.Fatalf("first import payload = %#v, want added=1 skipped=0", first)
	}

	second := importUsagePayload()
	if second["added"] != 0 || second["skipped"] != 1 {
		t.Fatalf("second import payload = %#v, want added=0 skipped=1", second)
	}

	page, err := stats.QueryEvents(usage.EventQuery{Limit: 10})
	if err != nil {
		t.Fatalf("QueryEvents returned error: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(page.Items))
	}
	if stats.SummarySnapshot().TotalRequests != 1 {
		t.Fatalf("summary total_requests = %d, want 1", stats.SummarySnapshot().TotalRequests)
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
