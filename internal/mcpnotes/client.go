// Package mcpnotes wraps the official MCP Go SDK's Streamable HTTP client to
// fetch todos from xaa.dev's ToDo MCP Server and reshape them into a Note
// model — same "there's no Notes MCP Server, so reshape ToDo data" approach
// as the TypeScript port's mcpNotesService.ts.
package mcpnotes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"oktane-go-mcp-xaa/internal/config"
)

type Note struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"createdAt,omitempty"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
}

// rawTodo is the shape xaa.dev's ToDo MCP Server actually returns.
type rawTodo struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Completed   bool     `json:"completed"`
	Priority    string   `json:"priority"`
	Tags        []string `json:"tags"`
	DueDate     string   `json:"dueDate"`
	Description string   `json:"description"`
}

type FetchResult struct {
	Resources  []string
	Notes      []Note
	RawContent string
}

// bearerTransport injects a static Authorization header — the go-sdk's
// StreamableClientTransport has no dedicated header field, so this is the
// documented way to attach one via a custom http.Client.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func Fetch(ctx context.Context, cfg config.XaaConfig, accessToken string) (*FetchResult, error) {
	httpClient := &http.Client{Transport: &bearerTransport{token: accessToken}}

	client := mcp.NewClient(&mcp.Implementation{Name: "oktane-go-mcp-xaa", Version: "1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: cfg.McpServerURL, HTTPClient: httpClient}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting to MCP server: %w", err)
	}
	defer session.Close()

	resourceList, err := session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		return nil, fmt.Errorf("listing resources: %w", err)
	}
	resources := make([]string, 0, len(resourceList.Resources))
	for _, r := range resourceList.Resources {
		resources = append(resources, r.URI)
	}

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: cfg.ResourceURI})
	if err != nil {
		return nil, fmt.Errorf("reading resource %s: %w", cfg.ResourceURI, err)
	}

	var sb strings.Builder
	for _, c := range result.Contents {
		sb.WriteString(c.Text)
	}
	raw := sb.String()

	return &FetchResult{Resources: resources, Notes: parseTodosAsNotes(raw), RawContent: raw}, nil
}

func toNote(t rawTodo) Note {
	return Note{ID: t.ID, Title: t.Title, Content: t.Description, Tags: t.Tags, UpdatedAt: t.DueDate}
}

// parseTodosAsNotes accepts either a raw JSON array of todos, or an object
// with a todos/notes/items/data/results array key — mirrors the fallback
// chain in the C#/TS ports' resource parsers.
func parseTodosAsNotes(raw string) []Note {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var arr []rawTodo
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return toNotes(arr)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}

	for _, key := range []string{"todos", "notes", "items", "data", "results"} {
		if raw2, ok := obj[key]; ok {
			var arr2 []rawTodo
			if err := json.Unmarshal(raw2, &arr2); err == nil {
				return toNotes(arr2)
			}
		}
	}
	return nil
}

func toNotes(todos []rawTodo) []Note {
	notes := make([]Note, 0, len(todos))
	for _, t := range todos {
		notes = append(notes, toNote(t))
	}
	return notes
}
