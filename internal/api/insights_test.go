package api

import (
	"testing"
	"time"

	"oktane-go-mcp-xaa/internal/mcpnotes"
)

func TestBuildInsights_Empty(t *testing.T) {
	ins := BuildInsights(nil)
	if ins.Summary != "No notes returned by the MCP server." {
		t.Fatalf("unexpected summary: %q", ins.Summary)
	}
	if ins.TotalCount != 0 {
		t.Fatalf("expected 0 total, got %d", ins.TotalCount)
	}
}

func TestBuildInsights_RecentAndTags(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	old := time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)

	notes := []mcpnotes.Note{
		{ID: "1", Title: "Recent note", Content: "hello world", Tags: []string{"work"}, UpdatedAt: now},
		{ID: "2", Title: "Old note", Content: "one two three", Tags: []string{"work", "home"}, UpdatedAt: old},
	}

	ins := BuildInsights(notes)
	if ins.TotalCount != 2 {
		t.Fatalf("expected 2 total, got %d", ins.TotalCount)
	}
	if ins.RecentCount != 1 {
		t.Fatalf("expected 1 recent, got %d", ins.RecentCount)
	}
	if ins.TopTag == nil || *ins.TopTag != "work" {
		t.Fatalf("expected top tag 'work', got %v", ins.TopTag)
	}
	if ins.WordCount != 5 {
		t.Fatalf("expected word count 5, got %d", ins.WordCount)
	}
	if len(ins.Items) == 0 {
		t.Fatal("expected at least one insight item")
	}
}
