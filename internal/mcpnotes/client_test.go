package mcpnotes

import "testing"

func TestParseTodosAsNotes_RawArray(t *testing.T) {
	raw := `[{"id":"1","title":"Buy milk","description":"2%","tags":["errand"],"dueDate":"2026-01-01"}]`
	notes := parseTodosAsNotes(raw)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Title != "Buy milk" || notes[0].Content != "2%" || notes[0].UpdatedAt != "2026-01-01" {
		t.Fatalf("unexpected note: %+v", notes[0])
	}
}

func TestParseTodosAsNotes_WrappedObject(t *testing.T) {
	raw := `{"todos":[{"id":"1","title":"A"},{"id":"2","title":"B"}]}`
	notes := parseTodosAsNotes(raw)
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
}

func TestParseTodosAsNotes_Empty(t *testing.T) {
	if notes := parseTodosAsNotes(""); notes != nil {
		t.Fatalf("expected nil for empty input, got %+v", notes)
	}
	if notes := parseTodosAsNotes("not json"); notes != nil {
		t.Fatalf("expected nil for invalid input, got %+v", notes)
	}
}
