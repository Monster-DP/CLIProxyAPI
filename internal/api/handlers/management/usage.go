package management

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

type usageExportPayload struct {
	Version    int                      `json:"version"`
	ExportedAt time.Time                `json:"exported_at"`
	Usage      usage.StatisticsSnapshot `json:"usage"`
}

type usageImportPayload struct {
	Version int                      `json:"version"`
	Usage   usage.StatisticsSnapshot `json:"usage"`
}

const currentUsageExportVersion = 2

// GetUsageStatistics returns the in-memory request statistics snapshot.
func (h *Handler) GetUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, gin.H{
		"usage":           snapshot,
		"failed_requests": snapshot.FailureCount,
	})
}

func (h *Handler) GetUsageEvents(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusOK, gin.H{"items": []any{}, "has_more": false, "total_matching": 0})
		return
	}

	query := usageEventQueryFromRequest(c)
	page, err := h.usageStats.QueryEvents(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]gin.H, 0, len(page.Items))
	for _, item := range page.Items {
		row := gin.H{
			"timestamp":                       item.Timestamp.UTC().Format(time.RFC3339Nano),
			"api_key":                         item.APIKey,
			"model":                           item.Model,
			"source":                          item.Source,
			"auth_index":                      item.AuthIndex,
			"failed":                          item.Failed,
			"tokens":                          item.Tokens,
			"event_cursor":                    item.EventCursor,
			"provider_first_token_latency_ms": item.ProviderFirstTokenLatencyMs,
			"first_token_latency_ms":          item.FirstTokenLatencyMs,
			"generation_duration_ms":          item.GenerationDurationMs,
			"precise_output_tps":              preciseOutputTpsForResponse(item),
		}
		if item.LatencyMs != 0 {
			row["latency_ms"] = item.LatencyMs
		}
		items = append(items, row)
	}

	nextBefore := ""
	if page.NextBefore != nil {
		nextBefore = page.NextBefore.Cursor
	}
	c.JSON(http.StatusOK, gin.H{
		"items":          items,
		"has_more":       page.HasMore,
		"next_before":    nextBefore,
		"total_matching": page.TotalMatching,
	})
}

func (h *Handler) GetUsageEventSummary(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusOK, gin.H{"source_items": []any{}, "model_items": []any{}, "total_matching": 0})
		return
	}

	query := usageEventQueryFromRequest(c)
	summary, err := h.usageStats.SummarizeEvents(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (h *Handler) GetUsageEventOptions(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusOK, gin.H{
			"sources":        []string{},
			"models":         []string{},
			"auth_indexes":   []string{},
			"total_matching": 0,
		})
		return
	}

	query := usageEventQueryFromRequest(c)
	options, err := h.usageStats.ListEventOptions(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, options)
}

// ExportUsageStatistics returns a complete usage snapshot for backup/migration.
func (h *Handler) ExportUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, usageExportPayload{
		Version:    currentUsageExportVersion,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
	})
}

// ImportUsageStatistics merges a previously exported usage snapshot into memory.
func (h *Handler) ImportUsageStatistics(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var payload usageImportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if payload.Version != 0 && payload.Version != 1 && payload.Version != currentUsageExportVersion {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported version"})
		return
	}
	if payload.Version <= 1 {
		payload.Usage = usage.NormalizeLegacyZeroStreamingTimings(payload.Usage)
	}

	result := h.usageStats.MergeSnapshot(payload.Usage)
	if err := h.usageStats.FlushPersistence(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to persist usage statistics",
			"added":   result.Added,
			"skipped": result.Skipped,
		})
		return
	}
	snapshot := h.usageStats.SummarySnapshot()
	c.JSON(http.StatusOK, gin.H{
		"added":           result.Added,
		"skipped":         result.Skipped,
		"total_requests":  snapshot.TotalRequests,
		"failed_requests": snapshot.FailureCount,
	})
}

func usageEventQueryFromRequest(c *gin.Context) usage.EventQuery {
	query := usage.EventQuery{
		Model:     c.Query("model"),
		Source:    c.Query("source"),
		AuthIndex: c.Query("auth_index"),
	}
	if before := c.Query("before"); before != "" {
		query.Before = &usage.EventQueryCursor{Cursor: before}
	}
	if limit, err := strconv.Atoi(c.DefaultQuery("limit", "100")); err == nil && limit > 0 {
		if limit > 1000 {
			limit = 1000
		}
		query.Limit = limit
	}

	now := time.Now().UTC()
	switch c.DefaultQuery("range", "24h") {
	case "7h":
		start := now.Add(-7 * time.Hour)
		query.Start = &start
	case "24h":
		start := now.Add(-24 * time.Hour)
		query.Start = &start
	case "7d":
		start := now.Add(-7 * 24 * time.Hour)
		query.Start = &start
	case "all":
	default:
		start := now.Add(-24 * time.Hour)
		query.Start = &start
	}

	return query
}

func preciseOutputTpsForResponse(item usage.UsageEvent) float64 {
	if item.GenerationDurationMs <= 0 || item.Tokens.OutputTokens <= 0 {
		return 0
	}
	return float64(item.Tokens.OutputTokens) * 1000 / float64(item.GenerationDurationMs)
}
