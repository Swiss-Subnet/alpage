package nns

import (
	"strings"
	"testing"
)

func renderTable(tb Table) string {
	var b strings.Builder
	tb.Render(&b)
	return b.String()
}

func TestTableAlignsColumns(t *testing.T) {
	tb := Table{
		Header: "nodes",
		Rows: []Row{
			{Status: "ok", Name: "short", ID: "id-1"},
			{Status: "operator-mismatch", Name: "a-much-longer-name", ID: "id-2"},
		},
	}
	lines := strings.Split(strings.TrimRight(renderTable(tb), "\n"), "\n")
	if lines[0] != "nodes" {
		t.Fatalf("first line = %q, want the header", lines[0])
	}
	// Both id columns start at the same offset once status and name are padded.
	if a, b := strings.Index(lines[1], "id-1"), strings.Index(lines[2], "id-2"); a != b {
		t.Errorf("id column not aligned: %d vs %d\n%s", a, b, renderTable(tb))
	}
}

// The old renderers hardcoded the status width, each sitting exactly at its
// longest status, so any longer status silently broke alignment. Width is now
// measured.
func TestTableWidthGrowsWithLongestStatus(t *testing.T) {
	tb := Table{
		Header: "nodes",
		Rows: []Row{
			{Status: "ok", Name: "n", ID: "id-1"},
			{Status: "a-status-far-longer-than-any-current-one", Name: "n", ID: "id-2"},
		},
	}
	lines := strings.Split(strings.TrimRight(renderTable(tb), "\n"), "\n")
	if a, b := strings.Index(lines[1], "id-1"), strings.Index(lines[2], "id-2"); a != b {
		t.Errorf("a longer status must widen the column: %d vs %d\n%s", a, b, renderTable(tb))
	}
}

func TestTableRendersDetailAndMissingName(t *testing.T) {
	tb := Table{
		Header: "nodes",
		Rows:   []Row{{Status: "gone", ID: "id-1", Detail: "  (why)"}},
	}
	got := renderTable(tb)
	if !strings.Contains(got, "  -  ") {
		t.Errorf("an empty name should render as -, got %q", got)
	}
	if !strings.Contains(got, "(why)") {
		t.Errorf("detail should be rendered, got %q", got)
	}
}

// An empty table prints its header plus the caller's empty note, so a section
// never silently vanishes.
func TestTableEmpty(t *testing.T) {
	tb := Table{Header: "subnet x", Empty: "  (no declared or live members)\n"}
	got := renderTable(tb)
	if got != "subnet x\n  (no declared or live members)\n" {
		t.Errorf("got %q", got)
	}
}

// A table with no rows and no empty note renders nothing at all, which is how
// the node/provider sections behave.
func TestTableEmptyWithoutNoteRendersNothing(t *testing.T) {
	if got := renderTable(Table{Header: "nodes"}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestTableHasDrift(t *testing.T) {
	clean := Table{Rows: []Row{{Status: "ok", OK: true}, {Status: "decommissioned", OK: true}}}
	if clean.HasDrift() {
		t.Error("all-OK rows are not drift")
	}
	dirty := Table{Rows: []Row{{Status: "ok", OK: true}, {Status: "gone"}}}
	if !dirty.HasDrift() {
		t.Error("a non-OK row is drift")
	}
}
