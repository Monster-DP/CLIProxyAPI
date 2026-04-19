package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type UsageEvent struct {
	APIKey                      string     `json:"api_key"`
	Model                       string     `json:"model"`
	Timestamp                   time.Time  `json:"timestamp"`
	LatencyMs                   int64      `json:"latency_ms,omitempty"`
	ProviderFirstTokenLatencyMs int64      `json:"provider_first_token_latency_ms,omitempty"`
	FirstTokenLatencyMs         int64      `json:"first_token_latency_ms,omitempty"`
	GenerationDurationMs        int64      `json:"generation_duration_ms,omitempty"`
	Source                      string     `json:"source,omitempty"`
	AuthIndex                   string     `json:"auth_index,omitempty"`
	Tokens                      TokenStats `json:"tokens"`
	Failed                      bool       `json:"failed"`
	EventCursor                 string     `json:"event_cursor"`
}

type EventQueryCursor struct {
	Cursor string `json:"cursor"`
}

type EventQuery struct {
	Start     *time.Time
	End       *time.Time
	Model     string
	Source    string
	AuthIndex string
	Limit     int
	Before    *EventQueryCursor
}

type EventPage struct {
	Items         []UsageEvent      `json:"items"`
	HasMore       bool              `json:"has_more"`
	TotalMatching int               `json:"total_matching"`
	NextBefore    *EventQueryCursor `json:"next_before,omitempty"`
}

var errStopEventArchiveScan = errors.New("stop event archive scan")

type EventArchive struct {
	rootDir string
	mu      sync.Mutex
}

type archiveFileSnapshot struct {
	path string
	size int64
}

func NewEventArchive(rootDir string) (*EventArchive, error) {
	trimmed := strings.TrimSpace(rootDir)
	if trimmed == "" {
		return nil, fmt.Errorf("event archive path is empty")
	}
	if err := os.MkdirAll(trimmed, 0o755); err != nil {
		return nil, fmt.Errorf("create event archive directory: %w", err)
	}
	return &EventArchive{rootDir: trimmed}, nil
}

func (a *EventArchive) Append(event UsageEvent) error {
	if a == nil {
		return fmt.Errorf("event archive is nil")
	}
	if event.Timestamp.IsZero() {
		return fmt.Errorf("event timestamp is required")
	}
	if event.EventCursor == "" {
		event.EventCursor = usageEventCursor(event)
	}

	path := filepath.Join(a.rootDir, event.Timestamp.UTC().Format("2006-01-02")+".jsonl")

	a.mu.Lock()
	defer a.mu.Unlock()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open event archive file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(event); err != nil {
		return fmt.Errorf("append usage event: %w", err)
	}
	return nil
}

func (a *EventArchive) Query(query EventQuery) (EventPage, error) {
	result := EventPage{}
	if a == nil {
		return result, fmt.Errorf("event archive is nil")
	}

	files, err := a.snapshotFilesForQuery(query)
	if err != nil {
		return result, err
	}
	beforeEvent, found, err := a.resolveBeforeEvent(query.Before, files)
	if err != nil {
		return result, err
	}
	if query.Before != nil && query.Before.Cursor != "" && !found {
		return result, nil
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	events, totalMatching, err := a.selectPageItems(eventQueryWithoutPagination(query), files, beforeEvent, limit)
	if err != nil {
		return result, err
	}
	result.Items = events
	result.TotalMatching = totalMatching
	result.HasMore = totalMatching > len(events)
	if len(result.Items) > 0 {
		result.NextBefore = &EventQueryCursor{Cursor: result.Items[len(result.Items)-1].EventCursor}
	}
	return result, nil
}

func (a *EventArchive) readMatchingEvents(query EventQuery) ([]UsageEvent, error) {
	events := make([]UsageEvent, 0)
	if err := a.scanMatchingEvents(query, func(event UsageEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		return nil, err
	}
	return events, nil
}

func (a *EventArchive) scanMatchingEvents(query EventQuery, visit func(UsageEvent) error) error {
	files, err := a.snapshotFilesForQuery(query)
	if err != nil {
		return err
	}
	return a.scanMatchingEventsFromFiles(query, files, visit)
}

func (a *EventArchive) snapshotFilesForQuery(query EventQuery) ([]archiveFileSnapshot, error) {
	files, err := a.snapshotFiles()
	if err != nil {
		return nil, err
	}
	return filterArchiveFilesForQuery(files, query), nil
}

func (a *EventArchive) snapshotFiles() ([]archiveFileSnapshot, error) {
	paths, err := filepath.Glob(filepath.Join(a.rootDir, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("glob event archive files: %w", err)
	}
	sort.Strings(paths)

	files := make([]archiveFileSnapshot, 0, len(paths))
	for _, path := range paths {
		info, errStat := os.Stat(path)
		if errStat != nil {
			return nil, fmt.Errorf("stat event archive file: %w", errStat)
		}
		files = append(files, archiveFileSnapshot{path: path, size: info.Size()})
	}
	return files, nil
}

func (a *EventArchive) scanMatchingEventsFromFiles(query EventQuery, files []archiveFileSnapshot, visit func(UsageEvent) error) error {
	for _, fileSnapshot := range files {
		file, errOpen := os.Open(fileSnapshot.path)
		if errOpen != nil {
			return fmt.Errorf("open event archive file: %w", errOpen)
		}

		scanner := bufio.NewScanner(io.LimitReader(file, fileSnapshot.size))
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var event UsageEvent
			if errUnmarshal := json.Unmarshal([]byte(line), &event); errUnmarshal != nil {
				_ = file.Close()
				return fmt.Errorf("decode usage event: %w", errUnmarshal)
			}
			event.EventCursor = archiveEventCursor(fileSnapshot.path, lineNumber)
			if !matchesEventQuery(event, query) {
				continue
			}
			if errVisit := visit(event); errVisit != nil {
				_ = file.Close()
				return errVisit
			}
		}

		if errScan := scanner.Err(); errScan != nil {
			_ = file.Close()
			return fmt.Errorf("scan event archive file: %w", errScan)
		}
		if errClose := file.Close(); errClose != nil {
			return fmt.Errorf("close event archive file: %w", errClose)
		}
	}

	return nil
}

func matchesEventQuery(event UsageEvent, query EventQuery) bool {
	if query.Start != nil && event.Timestamp.Before(*query.Start) {
		return false
	}
	if query.End != nil && event.Timestamp.After(*query.End) {
		return false
	}
	if query.Model != "" && event.Model != query.Model {
		return false
	}
	if query.Source != "" && event.Source != query.Source {
		return false
	}
	if query.AuthIndex != "" && event.AuthIndex != query.AuthIndex {
		return false
	}
	return true
}

func usageEventCursor(event UsageEvent) string {
	return fmt.Sprintf(
		"%s|%s|%s|%s|%s|%t|%d|%d|%d|%d|%d|%d|%d|%d|%d",
		event.Timestamp.UTC().Format(time.RFC3339Nano),
		event.APIKey,
		event.Model,
		event.Source,
		event.AuthIndex,
		event.Failed,
		event.LatencyMs,
		event.ProviderFirstTokenLatencyMs,
		event.FirstTokenLatencyMs,
		event.GenerationDurationMs,
		event.Tokens.InputTokens,
		event.Tokens.OutputTokens,
		event.Tokens.ReasoningTokens,
		event.Tokens.CachedTokens,
		event.Tokens.TotalTokens,
	)
}

func archiveEventCursor(path string, lineNumber int) string {
	return fmt.Sprintf("%s:%012d", filepath.Base(path), lineNumber)
}

func filterArchiveFilesForQuery(files []archiveFileSnapshot, query EventQuery) []archiveFileSnapshot {
	if len(files) == 0 {
		return files
	}
	startDay := eventQueryDay(query.Start)
	endDay := eventQueryDay(query.End)
	filtered := make([]archiveFileSnapshot, 0, len(files))
	for _, file := range files {
		day := archiveSnapshotDay(file)
		if day == "" {
			filtered = append(filtered, file)
			continue
		}
		if startDay != "" && day < startDay {
			continue
		}
		if endDay != "" && day > endDay {
			continue
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func archiveSnapshotDay(file archiveFileSnapshot) string {
	name := filepath.Base(file.path)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func eventQueryDay(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}

func (a *EventArchive) resolveBeforeEvent(before *EventQueryCursor, files []archiveFileSnapshot) (*UsageEvent, bool, error) {
	if before == nil || before.Cursor == "" {
		return nil, false, nil
	}

	var foundEvent UsageEvent
	err := a.scanMatchingEventsFromFiles(EventQuery{}, files, func(event UsageEvent) error {
		if event.EventCursor != before.Cursor {
			return nil
		}
		foundEvent = event
		return errStopEventArchiveScan
	})
	if err != nil {
		if errors.Is(err, errStopEventArchiveScan) {
			return &foundEvent, true, nil
		}
		return nil, false, err
	}
	return nil, false, nil
}

func (a *EventArchive) selectPageItems(query EventQuery, files []archiveFileSnapshot, beforeEvent *UsageEvent, limit int) ([]UsageEvent, int, error) {
	if limit <= 0 {
		limit = 100
	}
	files = filterArchiveFilesForBefore(files, beforeEvent)
	selection := make(eventSelectionHeap, 0, limit)
	totalMatching := 0
	err := a.scanMatchingEventsFromFiles(query, files, func(event UsageEvent) error {
		if beforeEvent != nil && compareUsageEventSort(event, *beforeEvent) <= 0 {
			return nil
		}
		totalMatching++
		selection.Add(event, limit)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	items := selection.Items()
	return items, totalMatching, nil
}

func filterArchiveFilesForBefore(files []archiveFileSnapshot, beforeEvent *UsageEvent) []archiveFileSnapshot {
	if beforeEvent == nil || len(files) == 0 {
		return files
	}
	beforeDay := beforeEvent.Timestamp.UTC().Format("2006-01-02")
	filtered := make([]archiveFileSnapshot, 0, len(files))
	for _, file := range files {
		day := archiveSnapshotDay(file)
		if day == "" || day <= beforeDay {
			filtered = append(filtered, file)
		}
	}
	return filtered
}
