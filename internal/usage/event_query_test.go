package usage

import (
	"path/filepath"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatisticsListEventOptionsIncludesArchivedValuesBeyondFirstPage(t *testing.T) {
	t.Parallel()

	previousEnabled := StatisticsEnabled()
	SetStatisticsEnabled(true)
	t.Cleanup(func() {
		SetStatisticsEnabled(previousEnabled)
	})

	stats := NewRequestStatistics()
	path := filepath.Join(t.TempDir(), "usage-statistics.json")
	if err := stats.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence returned error: %v", err)
	}

	baseTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	for i := range 510 {
		stats.Record(nil, coreusage.Record{
			APIKey:      "api-1",
			Model:       "gpt-5.4",
			RequestedAt: baseTime.Add(-time.Duration(i) * time.Minute),
			Source:      "provider-a",
			AuthIndex:   "codex:0",
			Detail: coreusage.Detail{
				InputTokens:  10,
				OutputTokens: 20,
				TotalTokens:  30,
			},
		})
	}

	stats.Record(nil, coreusage.Record{
		APIKey:      "api-2",
		Model:       "gpt-4.1",
		RequestedAt: baseTime.Add(-600 * time.Minute),
		Source:      "provider-z",
		AuthIndex:   "openai:1",
		Detail: coreusage.Detail{
			InputTokens:  5,
			OutputTokens: 8,
			TotalTokens:  13,
		},
	})

	options, err := stats.ListEventOptions(EventQuery{})
	if err != nil {
		t.Fatalf("ListEventOptions returned error: %v", err)
	}

	if options.TotalMatching != 511 {
		t.Fatalf("total_matching = %d, want 511", options.TotalMatching)
	}
	if len(options.Sources) != 2 {
		t.Fatalf("sources len = %d, want 2", len(options.Sources))
	}
	if options.Sources[0] != "provider-a" || options.Sources[1] != "provider-z" {
		t.Fatalf("sources = %#v, want [provider-a provider-z]", options.Sources)
	}
	if len(options.Models) != 2 {
		t.Fatalf("models len = %d, want 2", len(options.Models))
	}
	if options.Models[0] != "gpt-4.1" || options.Models[1] != "gpt-5.4" {
		t.Fatalf("models = %#v, want [gpt-4.1 gpt-5.4]", options.Models)
	}
	if len(options.AuthIndexes) != 2 {
		t.Fatalf("auth_indexes len = %d, want 2", len(options.AuthIndexes))
	}
	if options.AuthIndexes[0] != "codex:0" || options.AuthIndexes[1] != "openai:1" {
		t.Fatalf("auth_indexes = %#v, want [codex:0 openai:1]", options.AuthIndexes)
	}
}
