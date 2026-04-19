package usage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEventArchiveAppendAndQueryNewestFirst(t *testing.T) {
	t.Parallel()

	archive, err := NewEventArchive(filepath.Join(t.TempDir(), "usage-events"))
	if err != nil {
		t.Fatalf("NewEventArchive returned error: %v", err)
	}

	events := []UsageEvent{
		{
			APIKey:      "api-1",
			Model:       "gpt-5.4",
			Timestamp:   time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC),
			Source:      "provider-a",
			AuthIndex:   "codex:0",
			Tokens:      TokenStats{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
			EventCursor: "2026-04-15T09:00:00Z|provider-a|codex:0|gpt-5.4",
		},
		{
			APIKey:      "api-1",
			Model:       "gpt-5.4",
			Timestamp:   time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC),
			Source:      "provider-b",
			AuthIndex:   "codex:1",
			Tokens:      TokenStats{InputTokens: 11, OutputTokens: 21, TotalTokens: 32},
			EventCursor: "2026-04-16T09:00:00Z|provider-b|codex:1|gpt-5.4",
		},
		{
			APIKey:      "api-2",
			Model:       "gpt-4.1",
			Timestamp:   time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC),
			Source:      "provider-c",
			AuthIndex:   "openai:0",
			Tokens:      TokenStats{InputTokens: 12, OutputTokens: 22, TotalTokens: 34},
			EventCursor: "2026-04-17T09:00:00Z|provider-c|openai:0|gpt-4.1",
		},
	}

	for _, event := range events {
		if errAppend := archive.Append(event); errAppend != nil {
			t.Fatalf("Append returned error: %v", errAppend)
		}
	}

	page, err := archive.Query(EventQuery{Limit: 2})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	if page.TotalMatching != 3 {
		t.Fatalf("total_matching = %d, want 3", page.TotalMatching)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(page.Items))
	}
	if !page.HasMore {
		t.Fatal("has_more = false, want true")
	}
	if page.Items[0].Source != "provider-c" {
		t.Fatalf("first source = %q, want provider-c", page.Items[0].Source)
	}
	if page.Items[1].Source != "provider-b" {
		t.Fatalf("second source = %q, want provider-b", page.Items[1].Source)
	}
	if page.NextBefore == nil || page.NextBefore.Cursor == "" {
		t.Fatalf("next_before = %#v, want non-empty cursor", page.NextBefore)
	}
}

func TestEventArchiveQueryHonorsFiltersAndBeforeCursor(t *testing.T) {
	t.Parallel()

	archive, err := NewEventArchive(filepath.Join(t.TempDir(), "usage-events"))
	if err != nil {
		t.Fatalf("NewEventArchive returned error: %v", err)
	}

	baseTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	events := []UsageEvent{
		{
			APIKey:                      "api-1",
			Model:                       "gpt-5.4",
			Timestamp:                   baseTime.Add(-3 * time.Hour),
			Source:                      "provider-a",
			AuthIndex:                   "codex:0",
			ProviderFirstTokenLatencyMs: 500,
			FirstTokenLatencyMs:         700,
			GenerationDurationMs:        2500,
			Tokens:                      TokenStats{InputTokens: 10, OutputTokens: 25, TotalTokens: 35},
			EventCursor:                 "cursor-1",
		},
		{
			APIKey:                      "api-1",
			Model:                       "gpt-5.4",
			Timestamp:                   baseTime.Add(-2 * time.Hour),
			Source:                      "provider-a",
			AuthIndex:                   "codex:1",
			ProviderFirstTokenLatencyMs: 550,
			FirstTokenLatencyMs:         750,
			GenerationDurationMs:        2600,
			Tokens:                      TokenStats{InputTokens: 11, OutputTokens: 26, TotalTokens: 37},
			EventCursor:                 "cursor-2",
		},
		{
			APIKey:      "api-1",
			Model:       "gpt-4.1",
			Timestamp:   baseTime.Add(-1 * time.Hour),
			Source:      "provider-b",
			AuthIndex:   "openai:0",
			Tokens:      TokenStats{InputTokens: 12, OutputTokens: 27, TotalTokens: 39},
			EventCursor: "cursor-3",
		},
	}

	for _, event := range events {
		if errAppend := archive.Append(event); errAppend != nil {
			t.Fatalf("Append returned error: %v", errAppend)
		}
	}

	start := baseTime.Add(-150 * time.Minute)
	page, err := archive.Query(EventQuery{
		Start:     &start,
		Model:     "gpt-5.4",
		Source:    "provider-a",
		AuthIndex: "codex:1",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	if page.TotalMatching != 1 {
		t.Fatalf("total_matching = %d, want 1", page.TotalMatching)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(page.Items))
	}
	if page.Items[0].AuthIndex != "codex:1" {
		t.Fatalf("auth_index = %q, want codex:1", page.Items[0].AuthIndex)
	}
	if page.Items[0].EventCursor == "" {
		t.Fatal("event_cursor is empty")
	}

	page, err = archive.Query(EventQuery{
		Limit:  1,
		Before: page.NextBefore,
	})
	if err != nil {
		t.Fatalf("Query with before returned error: %v", err)
	}

	if len(page.Items) != 1 {
		t.Fatalf("before items len = %d, want 1", len(page.Items))
	}
	if page.Items[0].Source != "provider-a" {
		t.Fatalf("before source = %q, want provider-a", page.Items[0].Source)
	}
}

func TestEventArchiveQueryPaginatesDistinctRowsThatShareLegacyIdentity(t *testing.T) {
	t.Parallel()

	archive, err := NewEventArchive(filepath.Join(t.TempDir(), "usage-events"))
	if err != nil {
		t.Fatalf("NewEventArchive returned error: %v", err)
	}

	timestamp := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	for _, event := range []UsageEvent{
		{
			APIKey:                      "api-1",
			Model:                       "gpt-5.4",
			Timestamp:                   timestamp,
			Source:                      "provider-a",
			AuthIndex:                   "codex:0",
			LatencyMs:                   1200,
			ProviderFirstTokenLatencyMs: 350,
			FirstTokenLatencyMs:         200,
			GenerationDurationMs:        1800,
			Tokens:                      TokenStats{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
			EventCursor:                 "legacy-cursor",
		},
		{
			APIKey:                      "api-1",
			Model:                       "gpt-5.4",
			Timestamp:                   timestamp,
			Source:                      "provider-a",
			AuthIndex:                   "codex:0",
			LatencyMs:                   2400,
			ProviderFirstTokenLatencyMs: 800,
			FirstTokenLatencyMs:         700,
			GenerationDurationMs:        900,
			Tokens:                      TokenStats{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
			EventCursor:                 "legacy-cursor",
		},
		{
			APIKey:                      "api-1",
			Model:                       "gpt-5.4",
			Timestamp:                   timestamp,
			Source:                      "provider-a",
			AuthIndex:                   "codex:0",
			LatencyMs:                   3600,
			ProviderFirstTokenLatencyMs: 1000,
			FirstTokenLatencyMs:         950,
			GenerationDurationMs:        700,
			Tokens:                      TokenStats{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
			EventCursor:                 "legacy-cursor",
		},
	} {
		if errAppend := archive.Append(event); errAppend != nil {
			t.Fatalf("Append returned error: %v", errAppend)
		}
	}

	page1, err := archive.Query(EventQuery{Limit: 1})
	if err != nil {
		t.Fatalf("first Query returned error: %v", err)
	}
	if len(page1.Items) != 1 {
		t.Fatalf("page1 items len = %d, want 1", len(page1.Items))
	}

	page2, err := archive.Query(EventQuery{Limit: 1, Before: page1.NextBefore})
	if err != nil {
		t.Fatalf("second Query returned error: %v", err)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("page2 items len = %d, want 1", len(page2.Items))
	}

	page3, err := archive.Query(EventQuery{Limit: 1, Before: page2.NextBefore})
	if err != nil {
		t.Fatalf("third Query returned error: %v", err)
	}
	if len(page3.Items) != 1 {
		t.Fatalf("page3 items len = %d, want 1", len(page3.Items))
	}

	if page1.Items[0].EventCursor == page2.Items[0].EventCursor || page2.Items[0].EventCursor == page3.Items[0].EventCursor {
		t.Fatalf("expected unique cursors across pages, got %q %q %q", page1.Items[0].EventCursor, page2.Items[0].EventCursor, page3.Items[0].EventCursor)
	}
	if page1.Items[0].LatencyMs == page2.Items[0].LatencyMs || page2.Items[0].LatencyMs == page3.Items[0].LatencyMs {
		t.Fatalf("expected distinct rows across pages, got latencies %d %d %d", page1.Items[0].LatencyMs, page2.Items[0].LatencyMs, page3.Items[0].LatencyMs)
	}
}
