package nns

import (
	"strings"
	"testing"
)

// Synthetic throughout: the id and the tally are not a real mainnet proposal,
// and nothing here depends on them being one.
func terminalEntry() Entry {
	return Entry{
		ProposalID: 1,
		Status:     StateExecuted,
		ResolvedAt: "2026-09-01T10:00:00Z",
		Outcome: &Outcome{
			Label:  "Executed (4)",
			Source: SourceGovernance,
			Yes:    30,
			Total:  40,
		},
	}
}

// A settled proposal is reported from state rather than queried again, and the
// line has to be identical to the queried one. A shorter line would read as
// missing information instead of as a saved round trip.
func TestSettledStatusRendersTheSameLine(t *testing.T) {
	e := terminalEntry()
	cached := SettledStatus(e)
	if cached == nil {
		t.Fatal("a terminal entry with a recorded outcome must resolve from state")
	}
	fetched := &ProposalStatus{
		Source: SourceGovernance,
		Label:  "Executed (4)",
		State:  StateExecuted,
		Yes:    30,
		Total:  40,
	}
	if got, want := StatusLine("p", e, cached), StatusLine("p", e, fetched); got != want {
		t.Errorf("cached line differs from queried line:\n got %q\nwant %q", got, want)
	}
}

// Only a settled proposal can be served from state. Anything still moving has
// a tally that is still changing, so it must be queried.
func TestSettledStatusRefusesUnsettled(t *testing.T) {
	for _, st := range []ProposalState{StateOpen, StateAdopted, ProposalState("")} {
		e := terminalEntry()
		e.Status = st
		if SettledStatus(e) != nil {
			t.Errorf("state %q is not terminal and must be queried", st)
		}
	}
}

// Entries written before outcomes were recorded have a terminal status and no
// outcome, so they fall through to a query once and are cached after that.
func TestSettledStatusRefusesEntryWithoutOutcome(t *testing.T) {
	e := terminalEntry()
	e.Outcome = nil
	if SettledStatus(e) != nil {
		t.Error("an entry with no recorded outcome must be queried")
	}
}

// A dashboard-sourced outcome keeps its provenance, so a cached line still says
// governance has purged the proposal.
func TestSettledStatusKeepsDashboardProvenance(t *testing.T) {
	e := terminalEntry()
	e.Outcome.Source = SourceDashboard
	line := StatusLine("p", e, SettledStatus(e))
	if !strings.Contains(line, "purged from governance") {
		t.Errorf("a dashboard-sourced outcome must stay flagged, got %q", line)
	}
}

func TestRecordOutcomeOnlyOnceAndOnlyWhenTerminal(t *testing.T) {
	st := &State{Proposals: map[string]Entry{"p": {ProposalID: 1}}}

	if st.RecordOutcome("p", StateOpen, Outcome{Label: "Open (1)"}) {
		t.Error("an open proposal has no settled outcome to record")
	}
	if !st.RecordOutcome("p", StateExecuted, Outcome{Label: "Executed (4)"}) {
		t.Fatal("a terminal proposal must record its outcome")
	}
	// History does not get rewritten: a second observation of a settled
	// proposal must not overwrite what was recorded when it settled.
	if st.RecordOutcome("p", StateExecuted, Outcome{Label: "Failed (5)"}) {
		t.Error("a recorded outcome must not be overwritten")
	}
	if got := st.Proposals["p"].Outcome.Label; got != "Executed (4)" {
		t.Errorf("outcome label = %q, want the first recorded one", got)
	}
}

// The round trip through state has to preserve every field the line reads.
func TestOutcomeRoundTrip(t *testing.T) {
	ps := &ProposalStatus{
		Source:  SourceDashboard,
		Label:   "Failed (5)",
		State:   StateFailed,
		Yes:     10,
		No:      3,
		Total:   20,
		Failure: "canister trapped",
	}
	e := Entry{Status: StateFailed}
	o := ps.Outcome()
	e.Outcome = &o
	back := SettledStatus(e)
	if back.Label != ps.Label || back.Yes != ps.Yes || back.No != ps.No ||
		back.Total != ps.Total || back.Failure != ps.Failure || back.Source != ps.Source {
		t.Errorf("round trip lost fields:\n got %+v\nwant %+v", back, ps)
	}
}
