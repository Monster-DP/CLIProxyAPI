package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const defaultPersistenceFlushInterval = 200 * time.Millisecond
const defaultEventArchiveDirName = "usage-events"

type statisticsPersistence struct {
	path          string
	flushInterval time.Duration
	triggerCh     chan struct{}
	flushCh       chan chan error
	stopCh        chan struct{}
	stopOnce      sync.Once
}

func newStatisticsPersistence(path string) *statisticsPersistence {
	return &statisticsPersistence{
		path:          path,
		flushInterval: defaultPersistenceFlushInterval,
		triggerCh:     make(chan struct{}, 1),
		flushCh:       make(chan chan error),
		stopCh:        make(chan struct{}),
	}
}

// SaveSnapshotToFile writes a statistics snapshot to disk using an atomic replace.
func SaveSnapshotToFile(path string, snapshot StatisticsSnapshot) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return fmt.Errorf("usage statistics persistence path is empty")
	}

	dir := filepath.Dir(trimmedPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create usage statistics directory: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal usage statistics snapshot: %w", err)
	}
	data = append(data, '\n')

	tmpFile, err := os.CreateTemp(dir, "usage-statistics-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary usage statistics file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temporary usage statistics file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary usage statistics file: %w", err)
	}

	if err := os.Rename(tmpPath, trimmedPath); err != nil {
		if removeErr := os.Remove(trimmedPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("replace usage statistics file: %w", err)
		}
		if retryErr := os.Rename(tmpPath, trimmedPath); retryErr != nil {
			return fmt.Errorf("replace usage statistics file: %w", retryErr)
		}
	}

	return nil
}

// LoadSnapshotFromFile loads a statistics snapshot from disk.
func LoadSnapshotFromFile(path string) (StatisticsSnapshot, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return StatisticsSnapshot{}, fmt.Errorf("usage statistics persistence path is empty")
	}

	data, err := os.ReadFile(trimmedPath)
	if err != nil {
		return StatisticsSnapshot{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return StatisticsSnapshot{}, nil
	}

	var snapshot StatisticsSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return StatisticsSnapshot{}, fmt.Errorf("unmarshal usage statistics snapshot: %w", err)
	}
	snapshot = NormalizeLegacyZeroStreamingTimings(snapshot)

	return snapshot, nil
}

// EnablePersistence configures automatic statistics persistence and restores any existing snapshot.
func (s *RequestStatistics) EnablePersistence(path string) error {
	if s == nil {
		return nil
	}

	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return fmt.Errorf("usage statistics persistence path is empty")
	}
	if !filepath.IsAbs(trimmedPath) {
		absPath, err := filepath.Abs(trimmedPath)
		if err != nil {
			return fmt.Errorf("resolve usage statistics persistence path: %w", err)
		}
		trimmedPath = absPath
	}

	persistence := newStatisticsPersistence(trimmedPath)
	eventArchive, err := NewEventArchive(eventArchivePath(trimmedPath))
	if err != nil {
		return err
	}
	archiveFiles, err := eventArchive.snapshotFiles()
	if err != nil {
		return err
	}
	go persistence.run(s)

	s.mu.Lock()
	previousPersistence := s.persistence
	s.persistenceGen++
	currentGen := s.persistenceGen
	s.persistence = persistence
	s.eventArchive = eventArchive
	s.mu.Unlock()

	if previousPersistence != nil {
		previousPersistence.stop()
	}

	snapshot, err := LoadSnapshotFromFile(trimmedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			snapshot = StatisticsSnapshot{}
		} else {
			return err
		}
	}
	snapshot = backfillSummaryFromDetails(snapshot)

	s.restoreSummarySnapshot(snapshot)
	if snapshotHasDetails(snapshot) {
		if err = s.migrateLegacyDetails(snapshot); err != nil {
			return err
		}
		if err = persistence.flushNow(s.SummarySnapshot()); err != nil {
			return err
		}
	}
	s.startArchiveReconcile(snapshot, persistence, eventArchive, archiveFiles, currentGen)
	return nil
}

// FlushPersistence writes the current statistics snapshot to disk immediately.
func (s *RequestStatistics) FlushPersistence() error {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	persistence := s.persistence
	s.mu.RUnlock()
	if persistence == nil {
		return nil
	}

	return persistence.flushNow(s.SummarySnapshot())
}

// DisablePersistence detaches local statistics persistence immediately.
func (s *RequestStatistics) DisablePersistence() {
	if s == nil {
		return
	}

	s.mu.Lock()
	persistence := s.persistence
	s.persistenceGen++
	s.persistence = nil
	s.eventArchive = nil
	s.mu.Unlock()

	if persistence != nil {
		persistence.stop()
	}
}

func (s *RequestStatistics) schedulePersistenceSave(persistence *statisticsPersistence) {
	if s == nil || persistence == nil {
		return
	}
	persistence.schedule()
}

func (p *statisticsPersistence) run(stats *RequestStatistics) {
	var (
		timer  *time.Timer
		timerC <-chan time.Time
	)

	for {
		select {
		case <-p.triggerCh:
			if timer == nil {
				timer = time.NewTimer(p.flushInterval)
				timerC = timer.C
				continue
			}
			if !timer.Stop() {
				select {
				case <-timerC:
				default:
				}
			}
			timer.Reset(p.flushInterval)
		case done := <-p.flushCh:
			if timer != nil {
				if !timer.Stop() {
					select {
					case <-timerC:
					default:
					}
				}
				timer = nil
				timerC = nil
			}
			done <- SaveSnapshotToFile(p.path, stats.SummarySnapshot())
		case <-timerC:
			if err := SaveSnapshotToFile(p.path, stats.SummarySnapshot()); err != nil {
				log.WithError(err).Warn("failed to persist usage statistics snapshot")
			}
			timer = nil
			timerC = nil
		case <-p.stopCh:
			if timer != nil {
				if !timer.Stop() {
					select {
					case <-timerC:
					default:
					}
				}
			}
			return
		}
	}
}

func (p *statisticsPersistence) schedule() {
	select {
	case p.triggerCh <- struct{}{}:
	default:
	}
}

func (p *statisticsPersistence) flushNow(snapshot StatisticsSnapshot) error {
	done := make(chan error, 1)
	select {
	case p.flushCh <- done:
		return <-done
	case <-p.stopCh:
		return SaveSnapshotToFile(p.path, snapshot)
	}
}

func (p *statisticsPersistence) stop() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})
}

func eventArchivePath(summaryPath string) string {
	return filepath.Join(filepath.Dir(summaryPath), defaultEventArchiveDirName)
}

func (s *RequestStatistics) startArchiveReconcile(baseSnapshot StatisticsSnapshot, persistence *statisticsPersistence, archive *EventArchive, files []archiveFileSnapshot, generation uint64) {
	if s == nil || archive == nil || persistence == nil {
		return
	}
	if len(files) == 0 {
		return
	}

	go func() {
		archiveSummary, archiveEventCount, errBuild := buildSummarySnapshotFromFiles(archive, files)
		if errBuild != nil {
			log.WithError(errBuild).Warn("failed to rebuild usage summary from archive in background")
			return
		}
		if archiveEventCount == 0 {
			return
		}

		delta := subtractStatisticsSnapshots(archiveSummary, baseSnapshot)
		if !deltaHasChanges(delta) {
			return
		}
		if !s.applySummaryDelta(delta, persistence, archive, generation) {
			return
		}
		s.schedulePersistenceSave(persistence)
	}()
}

func (s *RequestStatistics) restoreSummarySnapshot(snapshot StatisticsSnapshot) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalRequests = snapshot.TotalRequests
	s.successCount = snapshot.SuccessCount
	s.failureCount = snapshot.FailureCount
	s.totalTokens = snapshot.TotalTokens

	s.apis = make(map[string]*apiStats, len(snapshot.APIs))
	for apiName, apiSnapshot := range snapshot.APIs {
		stats := &apiStats{
			TotalRequests: apiSnapshot.TotalRequests,
			TotalTokens:   apiSnapshot.TotalTokens,
			Models:        make(map[string]*modelStats, len(apiSnapshot.Models)),
		}
		for modelName, modelSnapshot := range apiSnapshot.Models {
			stats.Models[modelName] = &modelStats{
				TotalRequests: modelSnapshot.TotalRequests,
				TotalTokens:   modelSnapshot.TotalTokens,
			}
		}
		s.apis[apiName] = stats
	}

	s.requestsByDay = make(map[string]int64, len(snapshot.RequestsByDay))
	for dayKey, count := range snapshot.RequestsByDay {
		s.requestsByDay[dayKey] = count
	}

	s.requestsByHour = make(map[int]int64, len(snapshot.RequestsByHour))
	for hourKey, count := range snapshot.RequestsByHour {
		hour, err := strconv.Atoi(hourKey)
		if err != nil {
			continue
		}
		s.requestsByHour[hour] = count
	}

	s.tokensByDay = make(map[string]int64, len(snapshot.TokensByDay))
	for dayKey, count := range snapshot.TokensByDay {
		s.tokensByDay[dayKey] = count
	}

	s.tokensByHour = make(map[int]int64, len(snapshot.TokensByHour))
	for hourKey, count := range snapshot.TokensByHour {
		hour, err := strconv.Atoi(hourKey)
		if err != nil {
			continue
		}
		s.tokensByHour[hour] = count
	}
}

func (s *RequestStatistics) migrateLegacyDetails(snapshot StatisticsSnapshot) error {
	if s == nil || s.eventArchive == nil {
		return nil
	}

	existingCounts := make(map[string]int)
	if err := s.eventArchive.scanMatchingEvents(EventQuery{}, func(event UsageEvent) error {
		existingCounts[dedupKey(event.APIKey, event.Model, requestDetailFromUsageEvent(event))]++
		return nil
	}); err != nil {
		return err
	}

	for apiName, apiSnapshot := range snapshot.APIs {
		for modelName, modelSnapshot := range apiSnapshot.Models {
			for _, detail := range modelSnapshot.Details {
				event := UsageEvent{
					APIKey:                      apiName,
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
				key := dedupKey(apiName, modelName, detail)
				if existingCounts[key] > 0 {
					existingCounts[key]--
					continue
				}
				if errAppend := s.eventArchive.Append(event); errAppend != nil {
					return errAppend
				}
			}
		}
	}

	return nil
}

func buildSummarySnapshotFromArchive(archive *EventArchive) (StatisticsSnapshot, int, error) {
	if archive == nil {
		return StatisticsSnapshot{
			APIs:           make(map[string]APISnapshot),
			RequestsByDay:  make(map[string]int64),
			RequestsByHour: make(map[string]int64),
			TokensByDay:    make(map[string]int64),
			TokensByHour:   make(map[string]int64),
		}, 0, nil
	}

	files, err := archive.snapshotFiles()
	if err != nil {
		return StatisticsSnapshot{}, 0, err
	}
	return buildSummarySnapshotFromFiles(archive, files)
}

func buildSummarySnapshotFromFiles(archive *EventArchive, files []archiveFileSnapshot) (StatisticsSnapshot, int, error) {
	snapshot := StatisticsSnapshot{
		APIs:           make(map[string]APISnapshot),
		RequestsByDay:  make(map[string]int64),
		RequestsByHour: make(map[string]int64),
		TokensByDay:    make(map[string]int64),
		TokensByHour:   make(map[string]int64),
	}
	if archive == nil {
		return snapshot, 0, nil
	}

	totalEvents := 0
	if err := archive.scanMatchingEventsFromFiles(EventQuery{}, files, func(event UsageEvent) error {
		totalEvents++
		addUsageEventToSnapshot(&snapshot, event)
		return nil
	}); err != nil {
		return StatisticsSnapshot{}, 0, err
	}
	return snapshot, totalEvents, nil
}

func subtractStatisticsSnapshots(current, base StatisticsSnapshot) StatisticsSnapshot {
	delta := StatisticsSnapshot{
		APIs:           make(map[string]APISnapshot),
		RequestsByDay:  make(map[string]int64),
		RequestsByHour: make(map[string]int64),
		TokensByDay:    make(map[string]int64),
		TokensByHour:   make(map[string]int64),
	}

	delta.TotalRequests = positiveDiff(current.TotalRequests, base.TotalRequests)
	delta.SuccessCount = positiveDiff(current.SuccessCount, base.SuccessCount)
	delta.FailureCount = positiveDiff(current.FailureCount, base.FailureCount)
	delta.TotalTokens = positiveDiff(current.TotalTokens, base.TotalTokens)

	for apiKey, apiSnapshot := range current.APIs {
		baseAPISnapshot := base.APIs[apiKey]
		apiDelta := APISnapshot{
			TotalRequests: positiveDiff(apiSnapshot.TotalRequests, baseAPISnapshot.TotalRequests),
			TotalTokens:   positiveDiff(apiSnapshot.TotalTokens, baseAPISnapshot.TotalTokens),
			Models:        make(map[string]ModelSnapshot),
		}

		for modelName, modelSnapshot := range apiSnapshot.Models {
			baseModelSnapshot := baseAPISnapshot.Models[modelName]
			modelDelta := ModelSnapshot{
				TotalRequests: positiveDiff(modelSnapshot.TotalRequests, baseModelSnapshot.TotalRequests),
				TotalTokens:   positiveDiff(modelSnapshot.TotalTokens, baseModelSnapshot.TotalTokens),
			}
			if modelDelta.TotalRequests > 0 || modelDelta.TotalTokens > 0 {
				apiDelta.Models[modelName] = modelDelta
			}
		}

		if apiDelta.TotalRequests > 0 || apiDelta.TotalTokens > 0 || len(apiDelta.Models) > 0 {
			delta.APIs[apiKey] = apiDelta
		}
	}

	for dayKey, count := range current.RequestsByDay {
		if diff := positiveDiff(count, base.RequestsByDay[dayKey]); diff > 0 {
			delta.RequestsByDay[dayKey] = diff
		}
	}
	for hourKey, count := range current.RequestsByHour {
		if diff := positiveDiff(count, base.RequestsByHour[hourKey]); diff > 0 {
			delta.RequestsByHour[hourKey] = diff
		}
	}
	for dayKey, count := range current.TokensByDay {
		if diff := positiveDiff(count, base.TokensByDay[dayKey]); diff > 0 {
			delta.TokensByDay[dayKey] = diff
		}
	}
	for hourKey, count := range current.TokensByHour {
		if diff := positiveDiff(count, base.TokensByHour[hourKey]); diff > 0 {
			delta.TokensByHour[hourKey] = diff
		}
	}

	return delta
}

func deltaHasChanges(delta StatisticsSnapshot) bool {
	return delta.TotalRequests > 0 ||
		delta.SuccessCount > 0 ||
		delta.FailureCount > 0 ||
		delta.TotalTokens > 0 ||
		len(delta.APIs) > 0 ||
		len(delta.RequestsByDay) > 0 ||
		len(delta.RequestsByHour) > 0 ||
		len(delta.TokensByDay) > 0 ||
		len(delta.TokensByHour) > 0
}

func (s *RequestStatistics) applySummaryDelta(delta StatisticsSnapshot, persistence *statisticsPersistence, archive *EventArchive, generation uint64) bool {
	if s == nil {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.persistence != persistence || s.eventArchive != archive || s.persistenceGen != generation {
		return false
	}

	s.totalRequests += delta.TotalRequests
	s.successCount += delta.SuccessCount
	s.failureCount += delta.FailureCount
	s.totalTokens += delta.TotalTokens

	if s.apis == nil {
		s.apis = make(map[string]*apiStats)
	}
	for apiKey, apiDelta := range delta.APIs {
		stats := s.apis[apiKey]
		if stats == nil {
			stats = &apiStats{Models: make(map[string]*modelStats)}
			s.apis[apiKey] = stats
		} else if stats.Models == nil {
			stats.Models = make(map[string]*modelStats)
		}
		stats.TotalRequests += apiDelta.TotalRequests
		stats.TotalTokens += apiDelta.TotalTokens
		for modelName, modelDelta := range apiDelta.Models {
			modelStatsValue := stats.Models[modelName]
			if modelStatsValue == nil {
				modelStatsValue = &modelStats{}
				stats.Models[modelName] = modelStatsValue
			}
			modelStatsValue.TotalRequests += modelDelta.TotalRequests
			modelStatsValue.TotalTokens += modelDelta.TotalTokens
		}
	}

	if s.requestsByDay == nil {
		s.requestsByDay = make(map[string]int64)
	}
	for dayKey, count := range delta.RequestsByDay {
		s.requestsByDay[dayKey] += count
	}
	if s.requestsByHour == nil {
		s.requestsByHour = make(map[int]int64)
	}
	for hourKey, count := range delta.RequestsByHour {
		hour, err := strconv.Atoi(hourKey)
		if err != nil {
			continue
		}
		s.requestsByHour[hour] += count
	}
	if s.tokensByDay == nil {
		s.tokensByDay = make(map[string]int64)
	}
	for dayKey, count := range delta.TokensByDay {
		s.tokensByDay[dayKey] += count
	}
	if s.tokensByHour == nil {
		s.tokensByHour = make(map[int]int64)
	}
	for hourKey, count := range delta.TokensByHour {
		hour, err := strconv.Atoi(hourKey)
		if err != nil {
			continue
		}
		s.tokensByHour[hour] += count
	}

	return true
}

func positiveDiff(current, base int64) int64 {
	if current <= base {
		return 0
	}
	return current - base
}

func addUsageEventToSnapshot(snapshot *StatisticsSnapshot, event UsageEvent) {
	if snapshot == nil {
		return
	}

	apiKey := strings.TrimSpace(event.APIKey)
	if apiKey == "" {
		apiKey = "unknown"
	}
	modelName := strings.TrimSpace(event.Model)
	if modelName == "" {
		modelName = "unknown"
	}
	tokens := normaliseTokenStats(event.Tokens)

	snapshot.TotalRequests++
	if event.Failed {
		snapshot.FailureCount++
	} else {
		snapshot.SuccessCount++
	}
	snapshot.TotalTokens += tokens.TotalTokens

	apiSnapshot := snapshot.APIs[apiKey]
	if apiSnapshot.Models == nil {
		apiSnapshot.Models = make(map[string]ModelSnapshot)
	}
	apiSnapshot.TotalRequests++
	apiSnapshot.TotalTokens += tokens.TotalTokens

	modelSnapshot := apiSnapshot.Models[modelName]
	modelSnapshot.TotalRequests++
	modelSnapshot.TotalTokens += tokens.TotalTokens
	apiSnapshot.Models[modelName] = modelSnapshot
	snapshot.APIs[apiKey] = apiSnapshot

	dayKey := event.Timestamp.Format("2006-01-02")
	hourKey := formatHour(event.Timestamp.Hour())
	snapshot.RequestsByDay[dayKey]++
	snapshot.RequestsByHour[hourKey]++
	snapshot.TokensByDay[dayKey] += tokens.TotalTokens
	snapshot.TokensByHour[hourKey] += tokens.TotalTokens
}

func snapshotHasDetails(snapshot StatisticsSnapshot) bool {
	for _, apiSnapshot := range snapshot.APIs {
		for _, modelSnapshot := range apiSnapshot.Models {
			if len(modelSnapshot.Details) > 0 {
				return true
			}
		}
	}
	return false
}

func backfillSummaryFromDetails(snapshot StatisticsSnapshot) StatisticsSnapshot {
	if !snapshotHasDetails(snapshot) {
		return snapshot
	}

	requestsByDay := make(map[string]int64)
	requestsByHour := make(map[string]int64)
	tokensByDay := make(map[string]int64)
	tokensByHour := make(map[string]int64)

	var (
		totalRequests int64
		successCount  int64
		failureCount  int64
		totalTokens   int64
	)

	for apiName, apiSnapshot := range snapshot.APIs {
		var apiRequests int64
		var apiTokens int64
		if apiSnapshot.Models == nil {
			apiSnapshot.Models = make(map[string]ModelSnapshot)
		}
		for modelName, modelSnapshot := range apiSnapshot.Models {
			if len(modelSnapshot.Details) == 0 {
				apiSnapshot.Models[modelName] = modelSnapshot
				continue
			}

			var modelRequests int64
			var modelTokens int64
			for _, detail := range modelSnapshot.Details {
				tokens := normaliseTokenStats(detail.Tokens)
				modelRequests++
				modelTokens += tokens.TotalTokens
				if detail.Failed {
					failureCount++
				} else {
					successCount++
				}

				dayKey := detail.Timestamp.Format("2006-01-02")
				hourKey := formatHour(detail.Timestamp.Hour())
				requestsByDay[dayKey]++
				requestsByHour[hourKey]++
				tokensByDay[dayKey] += tokens.TotalTokens
				tokensByHour[hourKey] += tokens.TotalTokens
			}

			if modelSnapshot.TotalRequests == 0 {
				modelSnapshot.TotalRequests = modelRequests
			}
			if modelSnapshot.TotalTokens == 0 {
				modelSnapshot.TotalTokens = modelTokens
			}
			apiSnapshot.Models[modelName] = modelSnapshot

			apiRequests += modelRequests
			apiTokens += modelTokens
		}

		if apiSnapshot.TotalRequests == 0 {
			apiSnapshot.TotalRequests = apiRequests
		}
		if apiSnapshot.TotalTokens == 0 {
			apiSnapshot.TotalTokens = apiTokens
		}
		snapshot.APIs[apiName] = apiSnapshot

		totalRequests += apiRequests
		totalTokens += apiTokens
	}

	if snapshot.TotalRequests == 0 {
		snapshot.TotalRequests = totalRequests
	}
	if snapshot.TotalTokens == 0 {
		snapshot.TotalTokens = totalTokens
	}
	if snapshot.SuccessCount == 0 && successCount > 0 {
		snapshot.SuccessCount = successCount
	}
	if snapshot.FailureCount == 0 && failureCount > 0 {
		snapshot.FailureCount = failureCount
	}
	if len(snapshot.RequestsByDay) == 0 && len(requestsByDay) > 0 {
		snapshot.RequestsByDay = requestsByDay
	}
	if len(snapshot.RequestsByHour) == 0 && len(requestsByHour) > 0 {
		snapshot.RequestsByHour = requestsByHour
	}
	if len(snapshot.TokensByDay) == 0 && len(tokensByDay) > 0 {
		snapshot.TokensByDay = tokensByDay
	}
	if len(snapshot.TokensByHour) == 0 && len(tokensByHour) > 0 {
		snapshot.TokensByHour = tokensByHour
	}

	return snapshot
}
