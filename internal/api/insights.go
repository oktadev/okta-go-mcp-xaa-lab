// Package api hosts the HTTP handlers and insight-building logic ported
// from Program.cs (C#) / server.ts (TypeScript) — same event contract on
// /api/flow, same Note-shaped insights as the TS port's buildInsights().
package api

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"oktane-go-mcp-xaa/internal/mcpnotes"
)

type InsightItem struct {
	Icon string `json:"icon"`
	Text string `json:"text"`
}

type Insights struct {
	Summary     string        `json:"summary"`
	TotalCount  int           `json:"totalCount"`
	RecentCount int           `json:"recentCount"`
	TopTag      *string       `json:"topTag"`
	TagCount    int           `json:"tagCount"`
	WordCount   int           `json:"wordCount"`
	RecentNotes []string      `json:"recentNotes"`
	Items       []InsightItem `json:"items"`
}

type dated struct {
	note    mcpnotes.Note
	updated time.Time
	hasDate bool
}

func BuildInsights(notes []mcpnotes.Note) Insights {
	if len(notes) == 0 {
		return Insights{Summary: "No notes returned by the MCP server.", Items: []InsightItem{}}
	}

	now := time.Now()
	withDates := make([]dated, len(notes))
	for i, n := range notes {
		d := dated{note: n}
		if t, ok := parseTime(n.UpdatedAt); ok {
			d.updated, d.hasDate = t, true
		} else if t, ok := parseTime(n.CreatedAt); ok {
			d.updated, d.hasDate = t, true
		}
		withDates[i] = d
	}

	total := len(notes)
	recentCount := 0
	for _, d := range withDates {
		if d.hasDate && now.Sub(d.updated) <= 7*24*time.Hour {
			recentCount++
		}
	}

	tagCounts := map[string]int{}
	for _, n := range notes {
		for _, tag := range n.Tags {
			tagCounts[tag]++
		}
	}
	var topTag string
	best := 0
	for tag, count := range tagCounts {
		if count > best {
			best, topTag = count, tag
		}
	}

	wordCount := 0
	for _, n := range notes {
		wordCount += len(strings.Fields(n.Content))
	}

	sorted := append([]dated(nil), withDates...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].updated.After(sorted[j].updated) })
	limit := 3
	if len(sorted) < limit {
		limit = len(sorted)
	}
	recentNotes := make([]string, 0, limit)
	for _, d := range sorted[:limit] {
		label := d.note.Title
		if d.hasDate {
			label += " · updated " + d.updated.Format("Jan 2")
		}
		recentNotes = append(recentNotes, label)
	}

	summary := fmt.Sprintf("%d note%s on file.", total, plural(total))
	if recentCount > 0 {
		summary += fmt.Sprintf(" %d updated in the last 7 days.", recentCount)
	}

	var items []InsightItem
	if recentCount > 0 {
		items = append(items, InsightItem{Icon: "📝", Text: fmt.Sprintf("%d note(s) updated in the last week.", recentCount)})
	}
	if topTag != "" {
		items = append(items, InsightItem{Icon: "🏷️", Text: fmt.Sprintf("Most active tag: #%s.", topTag)})
	}
	if len(items) == 0 {
		items = append(items, InsightItem{Icon: "👍", Text: "Everything looks organized!"})
	}

	var topTagPtr *string
	if topTag != "" {
		topTagPtr = &topTag
	}

	return Insights{
		Summary:     summary,
		TotalCount:  total,
		RecentCount: recentCount,
		TopTag:      topTagPtr,
		TagCount:    len(tagCounts),
		WordCount:   wordCount,
		RecentNotes: recentNotes,
		Items:       items,
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
