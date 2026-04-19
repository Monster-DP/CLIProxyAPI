package usage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestSaveSnapshotToFileAndLoadSnapshotFromFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	want := StatisticsSnapshot{
		TotalRequests: 1,
		SuccessCount:  1,
		FailureCount:  0,
		TotalTokens:   30,
		APIs: map[string]APISnapshot{
			"test-key": {
				TotalRequests: 1,
				TotalTokens:   30,
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						TotalRequests: 1,
						TotalTokens:   30,
						Details: []RequestDetail{{
							Timestamp:                   time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
							LatencyMs:                   1200,
							ProviderFirstTokenLatencyMs: 300,
							FirstTokenLatencyMs:         450,
							GenerationDurationMs:        750,
							Source:                      "codex",
							AuthIndex:                   "codex:0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}

	if err := SaveSnapshotToFile(path, want); err != nil {
		t.Fatalf("SaveSnapshotToFile returned error: %v", err)
	}

	got, err := LoadSnapshotFromFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFromFile returned error: %v", err)
	}

	if got.TotalRequests != want.TotalRequests {
		t.Fatalf("total_requests = %d, want %d", got.TotalRequests, want.TotalRequests)
	}
	if got.TotalTokens != want.TotalTokens {
		t.Fatalf("total_tokens = %d, want %d", got.TotalTokens, want.TotalTokens)
	}
	if len(got.APIs["test-key"].Models["gpt-5.4"].Details) != 1 {
		t.Fatalf("details len = %d, want 1", len(got.APIs["test-key"].Models["gpt-5.4"].Details))
	}
	detail := got.APIs["test-key"].Models["gpt-5.4"].Details[0]
	if detail.ProviderFirstTokenLatencyMs != 300 {
		t.Fatalf("provider_first_token_latency_ms = %d, want 300", detail.ProviderFirstTokenLatencyMs)
	}
	if detail.FirstTokenLatencyMs != 450 {
		t.Fatalf("first_token_latency_ms = %d, want 450", detail.FirstTokenLatencyMs)
	}
	if detail.GenerationDurationMs != 750 {
		t.Fatalf("generation_duration_ms = %d, want 750", detail.GenerationDurationMs)
	}
}

func TestRequestStatisticsEnablePersistenceRestoresExistingSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	want := StatisticsSnapshot{
		TotalRequests: 1,
		SuccessCount:  1,
		TotalTokens:   30,
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}
	if err := SaveSnapshotToFile(path, want); err != nil {
		t.Fatalf("SaveSnapshotToFile returned error: %v", err)
	}

	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	got := stats.Snapshot()
	if got.TotalRequests != want.TotalRequests {
		t.Fatalf("total_requests = %d, want %d", got.TotalRequests, want.TotalRequests)
	}
	if got.TotalTokens != want.TotalTokens {
		t.Fatalf("total_tokens = %d, want %d", got.TotalTokens, want.TotalTokens)
	}
}

func TestRequestStatisticsEnablePersistenceRestoresCompactSummarySnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	want := StatisticsSnapshot{
		TotalRequests: 3,
		SuccessCount:  2,
		FailureCount:  1,
		TotalTokens:   90,
		APIs: map[string]APISnapshot{
			"summary-key": {
				TotalRequests: 3,
				TotalTokens:   90,
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						TotalRequests: 3,
						TotalTokens:   90,
					},
				},
			},
		},
		RequestsByDay: map[string]int64{"2026-04-17": 3},
		TokensByDay:   map[string]int64{"2026-04-17": 90},
	}
	if err := SaveSnapshotToFile(path, want); err != nil {
		t.Fatalf("SaveSnapshotToFile returned error: %v", err)
	}

	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	got := stats.SummarySnapshot()
	if got.TotalRequests != want.TotalRequests {
		t.Fatalf("total_requests = %d, want %d", got.TotalRequests, want.TotalRequests)
	}
	if got.SuccessCount != want.SuccessCount {
		t.Fatalf("success_count = %d, want %d", got.SuccessCount, want.SuccessCount)
	}
	if got.FailureCount != want.FailureCount {
		t.Fatalf("failure_count = %d, want %d", got.FailureCount, want.FailureCount)
	}
	if got.TotalTokens != want.TotalTokens {
		t.Fatalf("total_tokens = %d, want %d", got.TotalTokens, want.TotalTokens)
	}
	if got.APIs["summary-key"].Models["gpt-5.4"].TotalRequests != 3 {
		t.Fatalf("model total_requests = %d, want 3", got.APIs["summary-key"].Models["gpt-5.4"].TotalRequests)
	}
}

func TestRequestStatisticsEnablePersistenceRebuildsSummaryFromLegacyDetailsWhenCountersMissing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	legacy := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"legacy-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}
	if err := SaveSnapshotToFile(path, legacy); err != nil {
		t.Fatalf("SaveSnapshotToFile returned error: %v", err)
	}

	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	summary := stats.SummarySnapshot()
	if summary.TotalRequests != 1 {
		t.Fatalf("total_requests = %d, want 1", summary.TotalRequests)
	}
	if summary.TotalTokens != 30 {
		t.Fatalf("total_tokens = %d, want 30", summary.TotalTokens)
	}
	modelSnapshot := summary.APIs["legacy-key"].Models["gpt-5.4"]
	if modelSnapshot.TotalRequests != 1 {
		t.Fatalf("model total_requests = %d, want 1", modelSnapshot.TotalRequests)
	}
	if modelSnapshot.TotalTokens != 30 {
		t.Fatalf("model total_tokens = %d, want 30", modelSnapshot.TotalTokens)
	}
}

func TestRequestStatisticsFlushPersistenceWritesMergedSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	result := stats.MergeSnapshot(StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
							Tokens: TokenStats{
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

	if err := stats.FlushPersistence(); err != nil {
		t.Fatalf("FlushPersistence returned error: %v", err)
	}

	got, err := LoadSnapshotFromFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFromFile returned error: %v", err)
	}
	if got.TotalRequests != 1 {
		t.Fatalf("total_requests = %d, want 1", got.TotalRequests)
	}
}

func TestEnablePersistenceMigratesLegacyDetailsOnlyOnce(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	legacy := StatisticsSnapshot{
		TotalRequests: 1,
		SuccessCount:  1,
		TotalTokens:   30,
		APIs: map[string]APISnapshot{
			"legacy-key": {
				TotalRequests: 1,
				TotalTokens:   30,
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						TotalRequests: 1,
						TotalTokens:   30,
						Details: []RequestDetail{{
							Timestamp: time.Date(2026, 4, 8, 8, 0, 0, 0, time.UTC),
							Source:    "provider-a",
							AuthIndex: "codex:0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}
	if err := SaveSnapshotToFile(path, legacy); err != nil {
		t.Fatalf("SaveSnapshotToFile returned error: %v", err)
	}

	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("first EnablePersistence returned error: %v", err)
	}
	stats.DisablePersistence()

	archive, err := NewEventArchive(filepath.Join(filepath.Dir(path), "usage-events"))
	if err != nil {
		t.Fatalf("NewEventArchive returned error: %v", err)
	}
	page, err := archive.Query(EventQuery{Limit: 10})
	if err != nil {
		t.Fatalf("first archive Query returned error: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("first archive items len = %d, want 1", len(page.Items))
	}

	stats = NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("second EnablePersistence returned error: %v", err)
	}
	stats.DisablePersistence()

	page, err = archive.Query(EventQuery{Limit: 10})
	if err != nil {
		t.Fatalf("second archive Query returned error: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("second archive items len = %d, want 1", len(page.Items))
	}
}

func TestEnablePersistenceEventuallyRebuildsSummaryFromArchiveWhenSummaryIsStale(t *testing.T) {
	t.Parallel()

	previousEnabled := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() {
		SetStatisticsEnabled(previousEnabled)
	})

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	requestedAt := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "stale-key",
		Model:       "gpt-5.4",
		RequestedAt: requestedAt,
		Source:      "provider-a",
		AuthIndex:   "codex:0",
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})
	stats.DisablePersistence()

	if err := SaveSnapshotToFile(path, StatisticsSnapshot{}); err != nil {
		t.Fatalf("SaveSnapshotToFile returned error: %v", err)
	}

	restored := NewRequestStatistics()
	if err := restored.EnablePersistence(path); err != nil {
		t.Fatalf("restored EnablePersistence returned error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		summary := restored.SummarySnapshot()
		modelSnapshot := summary.APIs["stale-key"].Models["gpt-5.4"]
		if summary.TotalRequests == 1 && summary.TotalTokens == 30 && modelSnapshot.TotalRequests == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("summary not rebuilt in time: %+v", summary)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRequestStatisticsRecordPersistsSnapshotAsync(t *testing.T) {
	t.Parallel()

	previousEnabled := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() {
		SetStatisticsEnabled(previousEnabled)
	})

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "persisted-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  12,
			OutputTokens: 18,
			TotalTokens:  30,
		},
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		snapshot, err := LoadSnapshotFromFile(path)
		if err == nil {
			if snapshot.TotalRequests >= 1 {
				return
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for persisted snapshot at %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRequestStatisticsDisablePersistenceStopsWritingToDisk(t *testing.T) {
	t.Parallel()

	previousEnabled := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() {
		SetStatisticsEnabled(previousEnabled)
	})

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}
	stats.DisablePersistence()

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "disabled-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 4, 8, 9, 30, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  12,
			OutputTokens: 18,
			TotalTokens:  30,
		},
	})

	time.Sleep(300 * time.Millisecond)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected persistence to stay disabled, stat err = %v", err)
	}
}

func TestRequestStatisticsSummarySnapshotOmitsDetailsWhileArchiveKeepsEvents(t *testing.T) {
	t.Parallel()

	previousEnabled := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() {
		SetStatisticsEnabled(previousEnabled)
	})

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	requestedAt := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	stats.Record(context.Background(), coreusage.Record{
		APIKey:                    "archived-key",
		Model:                     "gpt-5.4",
		RequestedAt:               requestedAt,
		Latency:                   1500 * time.Millisecond,
		ProviderFirstTokenLatency: 600 * time.Millisecond,
		FirstTokenLatency:         900 * time.Millisecond,
		GenerationDuration:        2400 * time.Millisecond,
		Source:                    "provider-a",
		AuthIndex:                 "codex:0",
		Detail: coreusage.Detail{
			InputTokens:  12,
			OutputTokens: 18,
			TotalTokens:  30,
		},
	})

	summary := stats.SummarySnapshot()
	modelSnapshot := summary.APIs["archived-key"].Models["gpt-5.4"]
	if summary.TotalRequests != 1 {
		t.Fatalf("summary total_requests = %d, want 1", summary.TotalRequests)
	}
	if len(modelSnapshot.Details) != 0 {
		t.Fatalf("summary details len = %d, want 0", len(modelSnapshot.Details))
	}

	archive, err := NewEventArchive(filepath.Join(filepath.Dir(path), "usage-events"))
	if err != nil {
		t.Fatalf("NewEventArchive returned error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		page, errQuery := archive.Query(EventQuery{Limit: 10})
		if errQuery == nil && len(page.Items) == 1 {
			if page.Items[0].APIKey != "archived-key" {
				t.Fatalf("archived api_key = %q, want archived-key", page.Items[0].APIKey)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for archived event, last err=%v", errQuery)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFlushPersistenceWritesCompactSummaryWithoutEmbeddedDetails(t *testing.T) {
	t.Parallel()

	previousEnabled := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() {
		SetStatisticsEnabled(previousEnabled)
	})

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	stats := NewRequestStatistics()
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "compact-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 4, 17, 11, 0, 0, 0, time.UTC),
		Source:      "provider-b",
		AuthIndex:   "codex:1",
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	if err := stats.FlushPersistence(); err != nil {
		t.Fatalf("FlushPersistence returned error: %v", err)
	}

	snapshot, err := LoadSnapshotFromFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFromFile returned error: %v", err)
	}

	if snapshot.TotalRequests != 1 {
		t.Fatalf("summary file total_requests = %d, want 1", snapshot.TotalRequests)
	}
	if len(snapshot.APIs["compact-key"].Models["gpt-5.4"].Details) != 0 {
		t.Fatalf("summary file details len = %d, want 0", len(snapshot.APIs["compact-key"].Models["gpt-5.4"].Details))
	}
}

func TestLoadSnapshotFromFileConvertsLegacyZeroStreamingTimingsToUnknownSentinel(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	data := []byte(`{
  "total_requests": 1,
  "success_count": 1,
  "failure_count": 0,
  "total_tokens": 30,
  "apis": {
    "test-key": {
      "models": {
        "gpt-5.4": {
          "details": [
            {
              "timestamp": "2026-04-08T08:00:00Z",
              "latency_ms": 1200,
              "provider_first_token_latency_ms": 0,
              "first_token_latency_ms": 0,
              "generation_duration_ms": 0,
              "tokens": {
                "input_tokens": 10,
                "output_tokens": 20,
                "total_tokens": 30
              }
            }
          ]
        }
      }
    }
  }
}
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	snapshot, err := LoadSnapshotFromFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFromFile returned error: %v", err)
	}

	detail := snapshot.APIs["test-key"].Models["gpt-5.4"].Details[0]
	if detail.ProviderFirstTokenLatencyMs >= 0 {
		t.Fatalf("provider_first_token_latency_ms = %d, want negative unknown sentinel", detail.ProviderFirstTokenLatencyMs)
	}
	if detail.FirstTokenLatencyMs >= 0 {
		t.Fatalf("first_token_latency_ms = %d, want negative unknown sentinel", detail.FirstTokenLatencyMs)
	}
	if detail.GenerationDurationMs >= 0 {
		t.Fatalf("generation_duration_ms = %d, want negative unknown sentinel", detail.GenerationDurationMs)
	}
}
