package nns

import (
	"fmt"
	"strings"
)

// Row is one line of a reconcile report: a status, the thing it is about, and
// an optional trailing explanation. Every reconciler classifies into these, so
// they all render identically.
type Row struct {
	Status string
	Name   string
	ID     string
	// Detail is appended verbatim, including its own leading spaces, so a
	// reconciler controls its own phrasing.
	Detail string
	// Remedy is what clears this row, set from a Remedier. Rendering lives in
	// Render so every reconciler phrases it the same way, and a row that is not
	// drift drops it.
	Remedy string
	// OK marks a row that is not drift. Statuses differ per reconciler, so the
	// classifier decides rather than Table guessing from the string.
	OK bool
	// Color is the ANSI colour for the status cell; empty renders unstyled.
	Color string
}

// Remedier is implemented by whatever a reconciler classifies into, usually its
// status type, so every table can ask the same question: what clears this row.
// Returning an empty string means the row needs no action.
//
// TestEveryDriftStatusHasRemedy walks the known statuses and fails on a drift
// status that answers nothing, so adding one without a remedy is caught rather
// than shipping a row that says what is wrong and not what to do.
type Remedier interface {
	remedy() string
}

// remedyOf is nil-safe so a reconciler can pass a value it did not classify.
func remedyOf(r Remedier) string {
	if r == nil {
		return ""
	}
	return r.remedy()
}

// Table is a rendered reconcile section.
type Table struct {
	Header string
	Rows   []Row
	// Empty is written under the header when there are no rows. Left empty, a
	// row-less table renders nothing at all.
	Empty string
}

func (t Table) HasDrift() bool {
	for _, row := range t.Rows {
		if !row.OK {
			return true
		}
	}
	return false
}

// Render writes the section: header, then one padded line per row. Column
// widths are measured rather than fixed, so a longer status can never push the
// columns out of alignment.
func (t Table) Render(b *strings.Builder) {
	if len(t.Rows) == 0 {
		if t.Empty == "" {
			return
		}
		fmt.Fprintf(b, "%s\n", t.Header)
		b.WriteString(t.Empty)
		return
	}
	fmt.Fprintf(b, "%s\n", t.Header)
	statusW, nameW := 0, 0
	for _, row := range t.Rows {
		if n := len(row.Status); n > statusW {
			statusW = n
		}
		if n := len(rowName(row)); n > nameW {
			nameW = n
		}
	}
	for _, row := range t.Rows {
		status := colorize(fmt.Sprintf("%-*s", statusW, row.Status), row.Color)
		fmt.Fprintf(b, "  %s  %-*s  %s%s%s\n", status, nameW, rowName(row), row.ID, row.Detail, rowRemedy(row))
	}
}

// rowRemedy renders the remedy for a drift row only. A row that is already in
// sync has nothing to act on, and printing advice beside it would read as a
// problem where there is none.
func rowRemedy(row Row) string {
	if row.OK || row.Remedy == "" {
		return ""
	}
	return colorize(" fix: "+row.Remedy, ansiYellow)
}

func rowName(row Row) string {
	if row.Name == "" {
		return "-"
	}
	return row.Name
}
