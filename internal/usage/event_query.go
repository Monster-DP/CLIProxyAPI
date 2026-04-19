package usage

import (
	"container/heap"
	"fmt"
	"sort"
)

type EventSummaryItem struct {
	ID                      string  `json:"id"`
	Label                   string  `json:"label"`
	AverageTTFTMs           float64 `json:"average_ttft_ms"`
	AveragePreciseOutputTps float64 `json:"average_precise_output_tps"`
	SampleCount             int     `json:"sample_count"`
}

type EventSummary struct {
	SourceItems   []EventSummaryItem `json:"source_items"`
	ModelItems    []EventSummaryItem `json:"model_items"`
	TotalMatching int                `json:"total_matching"`
}

type EventOptions struct {
	Sources       []string `json:"sources"`
	Models        []string `json:"models"`
	AuthIndexes   []string `json:"auth_indexes"`
	TotalMatching int      `json:"total_matching"`
}

func (s *RequestStatistics) QueryEvents(query EventQuery) (EventPage, error) {
	if s == nil {
		return EventPage{}, nil
	}

	s.mu.RLock()
	archive := s.eventArchive
	s.mu.RUnlock()
	if archive != nil {
		return archive.Query(query)
	}

	events := usageEventsFromSnapshot(s.Snapshot())
	return buildEventPage(filterUsageEvents(events, query), query), nil
}

func (s *RequestStatistics) SummarizeEvents(query EventQuery) (EventSummary, error) {
	result := EventSummary{}
	if s == nil {
		return result, nil
	}

	sourceGroups := newEventSummaryGroups()
	modelGroups := newEventSummaryGroups()
	err := s.visitMatchingEvents(eventQueryWithoutPagination(query), func(event UsageEvent) error {
		result.TotalMatching++
		sourceGroups.Add(event.Source, event)
		modelGroups.Add(event.Model, event)
		return nil
	})
	if err != nil {
		return result, err
	}
	result.SourceItems = sourceGroups.Items()
	result.ModelItems = modelGroups.Items()
	return result, nil
}

func (s *RequestStatistics) ListEventOptions(query EventQuery) (EventOptions, error) {
	result := EventOptions{}
	if s == nil {
		return result, nil
	}

	options := newEventOptionsBuilder()
	err := s.visitMatchingEvents(eventQueryWithoutPagination(query), func(event UsageEvent) error {
		result.TotalMatching++
		options.Add(event)
		return nil
	})
	if err != nil {
		return result, err
	}
	result.Sources = options.Values(options.sources)
	result.Models = options.Values(options.models)
	result.AuthIndexes = options.Values(options.authIndexes)
	return result, nil
}

func usageEventsFromSnapshot(snapshot StatisticsSnapshot) []UsageEvent {
	events := make([]UsageEvent, 0)
	occurrences := make(map[string]int)
	for apiKey, apiSnapshot := range snapshot.APIs {
		for modelName, modelSnapshot := range apiSnapshot.Models {
			for _, detail := range modelSnapshot.Details {
				event := UsageEvent{
					APIKey:                      apiKey,
					Model:                       modelName,
					Timestamp:                   detail.Timestamp,
					LatencyMs:                   detail.LatencyMs,
					ProviderFirstTokenLatencyMs: detail.ProviderFirstTokenLatencyMs,
					FirstTokenLatencyMs:         detail.FirstTokenLatencyMs,
					GenerationDurationMs:        detail.GenerationDurationMs,
					Source:                      detail.Source,
					AuthIndex:                   detail.AuthIndex,
					Tokens:                      detail.Tokens,
					Failed:                      detail.Failed,
				}
				baseCursor := usageEventCursor(event)
				occurrences[baseCursor]++
				event.EventCursor = snapshotEventCursor(baseCursor, occurrences[baseCursor])
				events = append(events, event)
			}
		}
	}
	return events
}

func snapshotEventCursor(baseCursor string, occurrence int) string {
	return baseCursor + "|" + sortKeyIndex(occurrence)
}

func sortKeyIndex(value int) string {
	return fmt.Sprintf("%012d", value)
}

func filterUsageEvents(events []UsageEvent, query EventQuery) []UsageEvent {
	filtered := make([]UsageEvent, 0, len(events))
	for _, event := range events {
		if !matchesEventQuery(event, query) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func (s *RequestStatistics) visitMatchingEvents(query EventQuery, visit func(UsageEvent) error) error {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	archive := s.eventArchive
	s.mu.RUnlock()
	if archive != nil {
		return archive.scanMatchingEvents(query, visit)
	}

	snapshot := s.Snapshot()
	occurrences := make(map[string]int)
	for apiKey, apiSnapshot := range snapshot.APIs {
		for modelName, modelSnapshot := range apiSnapshot.Models {
			for _, detail := range modelSnapshot.Details {
				event := UsageEvent{
					APIKey:                      apiKey,
					Model:                       modelName,
					Timestamp:                   detail.Timestamp,
					LatencyMs:                   detail.LatencyMs,
					ProviderFirstTokenLatencyMs: detail.ProviderFirstTokenLatencyMs,
					FirstTokenLatencyMs:         detail.FirstTokenLatencyMs,
					GenerationDurationMs:        detail.GenerationDurationMs,
					Source:                      detail.Source,
					AuthIndex:                   detail.AuthIndex,
					Tokens:                      detail.Tokens,
					Failed:                      detail.Failed,
				}
				baseCursor := usageEventCursor(event)
				occurrences[baseCursor]++
				event.EventCursor = snapshotEventCursor(baseCursor, occurrences[baseCursor])
				if !matchesEventQuery(event, query) {
					continue
				}
				if err := visit(event); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func eventQueryWithoutPagination(query EventQuery) EventQuery {
	return EventQuery{
		Start:     query.Start,
		End:       query.End,
		Model:     query.Model,
		Source:    query.Source,
		AuthIndex: query.AuthIndex,
	}
}

func buildEventPage(events []UsageEvent, query EventQuery) EventPage {
	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].EventCursor > events[j].EventCursor
		}
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	if query.Before != nil && query.Before.Cursor != "" {
		beforeIndex := -1
		for idx := range events {
			if events[idx].EventCursor == query.Before.Cursor {
				beforeIndex = idx
				break
			}
		}
		if beforeIndex >= 0 {
			events = events[beforeIndex+1:]
		} else {
			events = nil
		}
	}

	page := EventPage{TotalMatching: len(events)}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if len(events) > limit {
		page.Items = events[:limit]
		page.HasMore = true
		page.NextBefore = &EventQueryCursor{Cursor: page.Items[len(page.Items)-1].EventCursor}
		return page
	}

	page.Items = events
	if len(page.Items) > 0 {
		page.NextBefore = &EventQueryCursor{Cursor: page.Items[len(page.Items)-1].EventCursor}
	}
	return page
}

type eventSummaryAggregate struct {
	id        string
	label     string
	ttftTotal float64
	ttftCount int
	tpsTotal  float64
	tpsCount  int
}

type eventSummaryGroups map[string]*eventSummaryAggregate

func newEventSummaryGroups() eventSummaryGroups {
	return make(map[string]*eventSummaryAggregate)
}

func (g eventSummaryGroups) Add(groupID string, event UsageEvent) {
	if groupID == "" {
		groupID = "-"
	}
	agg, ok := g[groupID]
	if !ok {
		agg = &eventSummaryAggregate{id: groupID, label: groupID}
		g[groupID] = agg
	}
	if event.ProviderFirstTokenLatencyMs >= 0 {
		agg.ttftTotal += float64(event.ProviderFirstTokenLatencyMs)
		agg.ttftCount++
	}
	if tps := preciseOutputTps(event); tps > 0 {
		agg.tpsTotal += tps
		agg.tpsCount++
	}
}

func (g eventSummaryGroups) Items() []EventSummaryItem {
	items := make([]EventSummaryItem, 0, len(g))
	for _, agg := range g {
		item := EventSummaryItem{
			ID:          agg.id,
			Label:       agg.label,
			SampleCount: agg.ttftCount,
		}
		if agg.ttftCount > 0 {
			item.AverageTTFTMs = agg.ttftTotal / float64(agg.ttftCount)
		}
		if agg.tpsCount > 0 {
			item.AveragePreciseOutputTps = agg.tpsTotal / float64(agg.tpsCount)
		}
		items = append(items, item)
	}
	sortSummaryItems(items)
	return items
}

func summarizeUsageEvents(events []UsageEvent, groupBy func(UsageEvent) string) []EventSummaryItem {
	type aggregate struct {
		id        string
		label     string
		ttftTotal float64
		ttftCount int
		tpsTotal  float64
		tpsCount  int
	}

	groups := make(map[string]*aggregate)
	for _, event := range events {
		groupID := groupBy(event)
		if groupID == "" {
			groupID = "-"
		}
		agg, ok := groups[groupID]
		if !ok {
			agg = &aggregate{id: groupID, label: groupID}
			groups[groupID] = agg
		}
		if event.ProviderFirstTokenLatencyMs >= 0 {
			agg.ttftTotal += float64(event.ProviderFirstTokenLatencyMs)
			agg.ttftCount++
		}
		if tps := preciseOutputTps(event); tps > 0 {
			agg.tpsTotal += tps
			agg.tpsCount++
		}
	}

	items := make([]EventSummaryItem, 0, len(groups))
	for _, agg := range groups {
		item := EventSummaryItem{
			ID:          agg.id,
			Label:       agg.label,
			SampleCount: agg.ttftCount,
		}
		if agg.ttftCount > 0 {
			item.AverageTTFTMs = agg.ttftTotal / float64(agg.ttftCount)
		}
		if agg.tpsCount > 0 {
			item.AveragePreciseOutputTps = agg.tpsTotal / float64(agg.tpsCount)
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].AverageTTFTMs != items[j].AverageTTFTMs {
			return items[i].AverageTTFTMs < items[j].AverageTTFTMs
		}
		if items[i].AveragePreciseOutputTps != items[j].AveragePreciseOutputTps {
			return items[i].AveragePreciseOutputTps > items[j].AveragePreciseOutputTps
		}
		if items[i].SampleCount != items[j].SampleCount {
			return items[i].SampleCount > items[j].SampleCount
		}
		return items[i].Label < items[j].Label
	})

	return items
}

func sortSummaryItems(items []EventSummaryItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].AverageTTFTMs != items[j].AverageTTFTMs {
			return items[i].AverageTTFTMs < items[j].AverageTTFTMs
		}
		if items[i].AveragePreciseOutputTps != items[j].AveragePreciseOutputTps {
			return items[i].AveragePreciseOutputTps > items[j].AveragePreciseOutputTps
		}
		if items[i].SampleCount != items[j].SampleCount {
			return items[i].SampleCount > items[j].SampleCount
		}
		return items[i].Label < items[j].Label
	})
}

type eventOptionsBuilder struct {
	sources     map[string]struct{}
	models      map[string]struct{}
	authIndexes map[string]struct{}
}

func newEventOptionsBuilder() eventOptionsBuilder {
	return eventOptionsBuilder{
		sources:     make(map[string]struct{}),
		models:      make(map[string]struct{}),
		authIndexes: make(map[string]struct{}),
	}
}

func (b eventOptionsBuilder) Add(event UsageEvent) {
	if event.Source != "" {
		b.sources[event.Source] = struct{}{}
	}
	if event.Model != "" {
		b.models[event.Model] = struct{}{}
	}
	if event.AuthIndex != "" {
		b.authIndexes[event.AuthIndex] = struct{}{}
	}
}

func (b eventOptionsBuilder) Values(source map[string]struct{}) []string {
	values := make([]string, 0, len(source))
	for value := range source {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

type eventSelectionHeap []UsageEvent

func (h eventSelectionHeap) Len() int { return len(h) }

func (h eventSelectionHeap) Less(i, j int) bool {
	return compareUsageEventSort(h[i], h[j]) > 0
}

func (h eventSelectionHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *eventSelectionHeap) Push(x any) {
	*h = append(*h, x.(UsageEvent))
}

func (h *eventSelectionHeap) Pop() any {
	old := *h
	last := len(old) - 1
	item := old[last]
	*h = old[:last]
	return item
}

func (h *eventSelectionHeap) Add(event UsageEvent, limit int) {
	if limit <= 0 {
		return
	}
	if h.Len() < limit {
		heap.Push(h, event)
		return
	}
	if compareUsageEventSort(event, (*h)[0]) >= 0 {
		return
	}
	heap.Pop(h)
	heap.Push(h, event)
}

func (h eventSelectionHeap) Items() []UsageEvent {
	items := make([]UsageEvent, len(h))
	copy(items, h)
	sort.Slice(items, func(i, j int) bool {
		return compareUsageEventSort(items[i], items[j]) < 0
	})
	return items
}

func compareUsageEventSort(left, right UsageEvent) int {
	if left.Timestamp.After(right.Timestamp) {
		return -1
	}
	if left.Timestamp.Before(right.Timestamp) {
		return 1
	}
	if left.EventCursor > right.EventCursor {
		return -1
	}
	if left.EventCursor < right.EventCursor {
		return 1
	}
	return 0
}

func preciseOutputTps(event UsageEvent) float64 {
	if event.GenerationDurationMs <= 0 {
		return 0
	}
	outputTokens := event.Tokens.OutputTokens
	if outputTokens <= 0 {
		return 0
	}
	return float64(outputTokens) * 1000 / float64(event.GenerationDurationMs)
}
