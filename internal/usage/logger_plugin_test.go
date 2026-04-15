package usage

import (
	"context"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatisticsRecordIncludesLatency(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:             "test-key",
		Model:              "gpt-5.4",
		RequestedAt:        time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Latency:            1500 * time.Millisecond,
		FirstTokenLatency:  400 * time.Millisecond,
		GenerationDuration: 1100 * time.Millisecond,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].LatencyMs != 1500 {
		t.Fatalf("latency_ms = %d, want 1500", details[0].LatencyMs)
	}
	if details[0].FirstTokenLatencyMs != 400 {
		t.Fatalf("first_token_latency_ms = %d, want 400", details[0].FirstTokenLatencyMs)
	}
	if details[0].GenerationDurationMs != 1100 {
		t.Fatalf("generation_duration_ms = %d, want 1100", details[0].GenerationDurationMs)
	}
}

func TestRequestStatisticsMergeSnapshotDedupIgnoresLatency(t *testing.T) {
	stats := NewRequestStatistics()
	timestamp := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	first := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp:            timestamp,
							LatencyMs:            0,
							FirstTokenLatencyMs:  200,
							GenerationDurationMs: 1800,
							Source:               "user@example.com",
							AuthIndex:            "0",
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
	second := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp:            timestamp,
							LatencyMs:            2500,
							FirstTokenLatencyMs:  900,
							GenerationDurationMs: 900,
							Source:               "user@example.com",
							AuthIndex:            "0",
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

	result := stats.MergeSnapshot(first)
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("first merge = %+v, want added=1 skipped=0", result)
	}

	result = stats.MergeSnapshot(second)
	if result.Added != 0 || result.Skipped != 1 {
		t.Fatalf("second merge = %+v, want added=0 skipped=1", result)
	}

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
}

func TestRequestStatisticsRecordPreservesUnknownTimingSentinels(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:             "test-key",
		Model:              "gpt-5.4",
		RequestedAt:        time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Latency:            1500 * time.Millisecond,
		FirstTokenLatency:  -1 * time.Millisecond,
		GenerationDuration: -1 * time.Millisecond,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].FirstTokenLatencyMs >= 0 {
		t.Fatalf("first_token_latency_ms = %d, want negative unknown sentinel", details[0].FirstTokenLatencyMs)
	}
	if details[0].GenerationDurationMs >= 0 {
		t.Fatalf("generation_duration_ms = %d, want negative unknown sentinel", details[0].GenerationDurationMs)
	}
}

func TestRequestStatisticsRecordRoundsPositiveStreamingTimingsUpToOneMillisecond(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:             "test-key",
		Model:              "gpt-5.4",
		RequestedAt:        time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Latency:            1500 * time.Millisecond,
		FirstTokenLatency:  500 * time.Microsecond,
		GenerationDuration: 750 * time.Microsecond,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].FirstTokenLatencyMs != 1 {
		t.Fatalf("first_token_latency_ms = %d, want 1", details[0].FirstTokenLatencyMs)
	}
	if details[0].GenerationDurationMs != 1 {
		t.Fatalf("generation_duration_ms = %d, want 1", details[0].GenerationDurationMs)
	}
}
