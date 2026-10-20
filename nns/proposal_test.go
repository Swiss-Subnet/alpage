package nns

import (
	"fmt"
	"strings"
	"testing"

	"github.com/swiss-subnet/alpage/gen/governance"
)

// The dry run is where a multi-line summary gets proofread, so the verbose
// render prints the stored body in full. A list of proposals keeps one line per
// proposal, so the compact render still truncates.
func TestVerboseRenderPrintsTheWholeSummary(t *testing.T) {
	title := "Summary fixture"
	summary := "First line.\n\nSecond paragraph with the detail.\n"
	pi := &governance.ProposalInfo{Proposal: &governance.Proposal{
		Title:   &title,
		Summary: summary,
	}}

	verbose := RenderVerbose(pi)
	if !strings.Contains(verbose, "Second paragraph with the detail.") {
		t.Errorf("verbose render dropped the body:\n%s", verbose)
	}
	if !strings.Contains(verbose, fmt.Sprintf("%d bytes", len(summary))) {
		t.Errorf("verbose render should report the stored byte count:\n%s", verbose)
	}
	if compact := Render(pi); strings.Contains(compact, "Second paragraph") {
		t.Errorf("compact render should stop at the first line:\n%s", compact)
	}
}
