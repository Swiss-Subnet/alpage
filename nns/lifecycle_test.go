package nns

import (
	"strings"
	"testing"
)

func TestTerminalStatuses(t *testing.T) {
	for _, s := range []ProposalState{StateExecuted, StateRejected, StateFailed} {
		if !s.Terminal() {
			t.Errorf("%s must be terminal", s)
		}
	}
	for _, s := range []ProposalState{StateOpen, StateAdopted, ProposalState("")} {
		if s.Terminal() {
			t.Errorf("%s must not be terminal", s)
		}
	}
}

// Governance's numeric status enum maps onto the persisted state.
func TestProposalStateFromGovernance(t *testing.T) {
	tests := []struct {
		status int32
		want   ProposalState
	}{
		{1, StateOpen},
		{2, StateRejected},
		{3, StateAdopted},
		{4, StateExecuted},
		{5, StateFailed},
		{99, ProposalState("")},
	}
	for _, tt := range tests {
		if got := stateFromGovernance(tt.status); got != tt.want {
			t.Errorf("status %d: got %q, want %q", tt.status, got, tt.want)
		}
	}
}

// The dashboard reports uppercase strings rather than the numeric enum.
func TestProposalStateFromDashboard(t *testing.T) {
	tests := []struct {
		label string
		want  ProposalState
	}{
		{"EXECUTED", StateExecuted},
		{"REJECTED", StateRejected},
		{"FAILED", StateFailed},
		{"OPEN", StateOpen},
		{"ADOPTED", StateAdopted},
		{"SOMETHING_ELSE", ProposalState("")},
	}
	for _, tt := range tests {
		if got := stateFromDashboard(tt.label); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.label, got, tt.want)
		}
	}
}

// A terminal proposal is immutable history: resubmitting under the same name
// would overwrite the record of what actually happened on-chain.
func TestApplyDecisionTerminalRefusedEvenWithForce(t *testing.T) {
	for _, s := range []ProposalState{StateExecuted, StateRejected, StateFailed} {
		st := &State{Proposals: map[string]Entry{
			"x": {ProposalID: 42, PayloadSHA256: "hash1", Status: s},
		}}
		for _, force := range []bool{false, true} {
			_, prev, err := st.ApplyDecision("x", "hash2", force)
			if err == nil {
				t.Errorf("%s force=%v: must refuse", s, force)
				continue
			}
			if !strings.Contains(err.Error(), string(s)) {
				t.Errorf("%s force=%v: error should name the state, got %v", s, force, err)
			}
			if prev == nil || prev.ProposalID != 42 {
				t.Errorf("%s force=%v: verdict must carry the prior entry", s, force)
			}
		}
	}
}

// A non-terminal recorded proposal keeps the existing force semantics.
func TestApplyDecisionOpenStillForceable(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, PayloadSHA256: "old", Status: StateOpen},
	}}
	d, _, err := st.ApplyDecision("x", "new", true)
	if err != nil {
		t.Fatalf("open proposal should stay forceable: %v", err)
	}
	if d != ApplyProceed {
		t.Errorf("got %v, want proceed", d)
	}
}

// Entries recorded before lifecycle tracking existed have no status and must
// keep behaving exactly as before.
func TestApplyDecisionEmptyStatusUnchanged(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, PayloadSHA256: "old"},
	}}
	if _, _, err := st.ApplyDecision("x", "new", false); err == nil {
		t.Error("drift without force must still error")
	}
	if _, _, err := st.ApplyDecision("x", "new", true); err != nil {
		t.Errorf("force must still override drift for untracked entries: %v", err)
	}
}

// resolved_at must be the on-chain resolution time, not when alp happened to
// look: the observation time drifts with every run and is not a fact about the
// proposal.
func TestResolvedAtFromGovernance(t *testing.T) {
	// Proposal 142931: proposed 2026-07-17T13:42:30Z, executed 2026-07-20T09:35:57Z.
	pi := piWithStatus(142931, 4)
	pi.ProposalTimestampSeconds = 1784295750
	pi.DecidedTimestampSeconds = 1784540154
	pi.ExecutedTimestampSeconds = 1784540157

	ps := statusFromGovernance(pi)

	if ps.ResolvedAt != "2026-07-20T09:35:57Z" {
		t.Errorf("executed proposal: got %q, want the on-chain execution time", ps.ResolvedAt)
	}
}

func TestResolvedAtFailedUsesFailedTimestamp(t *testing.T) {
	pi := piWithStatus(1, 5)
	pi.DecidedTimestampSeconds = 1784540154
	pi.FailedTimestampSeconds = 1784540200
	if got := statusFromGovernance(pi).ResolvedAt; got != "2026-07-20T09:36:40Z" {
		t.Errorf("failed proposal should use failed_timestamp, got %q", got)
	}
}

// A rejected proposal never executes; its resolution time is when it was decided.
func TestResolvedAtRejectedUsesDecidedTimestamp(t *testing.T) {
	pi := piWithStatus(1, 2)
	pi.DecidedTimestampSeconds = 1784540154
	if got := statusFromGovernance(pi).ResolvedAt; got != "2026-07-20T09:35:54Z" {
		t.Errorf("rejected proposal should use decided_timestamp, got %q", got)
	}
}

func TestResolvedAtOpenIsEmpty(t *testing.T) {
	pi := piWithStatus(1, 1)
	pi.ProposalTimestampSeconds = 1784295750
	if got := statusFromGovernance(pi).ResolvedAt; got != "" {
		t.Errorf("an open proposal has no resolution time, got %q", got)
	}
}

func TestSubmittedAtFromGovernance(t *testing.T) {
	pi := piWithStatus(142931, 4)
	pi.ProposalTimestampSeconds = 1784295750
	if got := statusFromGovernance(pi).SubmittedAt; got != "2026-07-17T13:42:30Z" {
		t.Errorf("got %q, want the on-chain proposal time", got)
	}
}

// The chain knows when the proposal was created; a locally-recorded guess (or a
// hand-written placeholder) is corrected from it.
func TestRecordSubmittedAtCorrectsPlaceholder(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, SubmittedAt: "2026-07-17T00:00:00Z"},
	}}
	if !st.RecordSubmittedAt("x", "2026-07-17T13:42:30Z") {
		t.Fatal("should correct a placeholder")
	}
	if got := st.Proposals["x"].SubmittedAt; got != "2026-07-17T13:42:30Z" {
		t.Errorf("got %q", got)
	}
	if st.RecordSubmittedAt("x", "2026-07-17T13:42:30Z") {
		t.Error("unchanged value should report no change")
	}
}

func TestRecordSubmittedAtEmptyNeverClears(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, SubmittedAt: "2026-07-17T13:42:30Z"},
	}}
	if st.RecordSubmittedAt("x", "") {
		t.Error("empty observation must not record")
	}
	if st.Proposals["x"].SubmittedAt == "" {
		t.Error("empty observation cleared a known value")
	}
}

func TestRecordState(t *testing.T) {
	st := &State{Proposals: map[string]Entry{"x": {ProposalID: 42}}}

	if !st.RecordState("x", StateOpen, "t1") {
		t.Fatal("first observation should record")
	}
	if got := st.Proposals["x"]; got.Status != StateOpen || got.ResolvedAt != "" {
		t.Errorf("non-terminal must not stamp resolved_at: %+v", got)
	}
	if st.RecordState("x", StateOpen, "t2") {
		t.Error("unchanged state should report no change")
	}
	if !st.RecordState("x", StateExecuted, "t3") {
		t.Fatal("transition to terminal should record")
	}
	if got := st.Proposals["x"]; got.Status != StateExecuted || got.ResolvedAt != "t3" {
		t.Errorf("terminal must stamp resolved_at: %+v", got)
	}
}

// Terminal state is a fact that never changes: neither a later observation nor
// an empty one (governance purge, failed query) may overwrite it.
func TestRecordStateTerminalIsMonotonic(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, Status: StateExecuted, ResolvedAt: "t1"},
	}}
	for _, s := range []ProposalState{StateOpen, StateRejected, ProposalState("")} {
		if st.RecordState("x", s, "t2") {
			t.Errorf("%q must not overwrite a terminal state", s)
		}
	}
	if got := st.Proposals["x"]; got.Status != StateExecuted || got.ResolvedAt != "t1" {
		t.Errorf("terminal entry mutated: %+v", got)
	}
}

func TestRecordStateEmptyNeverClears(t *testing.T) {
	st := &State{Proposals: map[string]Entry{"x": {ProposalID: 42, Status: StateOpen}}}
	if st.RecordState("x", "", "t") {
		t.Error("empty observation must not record")
	}
	if st.Proposals["x"].Status != StateOpen {
		t.Error("empty observation cleared a known state")
	}
}

func TestRecordStateUnknownName(t *testing.T) {
	st := &State{Proposals: map[string]Entry{}}
	if st.RecordState("nope", StateExecuted, "t") {
		t.Error("unknown proposal must not be recorded")
	}
	if len(st.Proposals) != 0 {
		t.Error("unknown proposal must not create an entry")
	}
}

func TestListLineTerminalDriftIsInert(t *testing.T) {
	e := Entry{ProposalID: 142931, PayloadSHA256: "old", Status: StateExecuted, ResolvedAt: "2026-07-21T16:01:08Z"}
	line := ListLine("wave1", e, "new")
	if !strings.Contains(line, "executed") {
		t.Errorf("terminal state must be named: %q", line)
	}
	if strings.Contains(line, "DRIFT") {
		t.Errorf("drift on an executed proposal is inert, not DRIFT: %q", line)
	}
}

// Drift on a proposal that is still open is actionable and must stay loud.
func TestListLineOpenDriftIsLoud(t *testing.T) {
	e := Entry{ProposalID: 142931, PayloadSHA256: "old", Status: StateOpen}
	line := ListLine("wave1", e, "new")
	if !strings.Contains(line, "DRIFT") {
		t.Errorf("drift on an open proposal must stay loud: %q", line)
	}
}

// Without a recorded status, drift keeps the original loud rendering.
func TestListLineUntrackedDriftIsLoud(t *testing.T) {
	e := Entry{ProposalID: 142931, PayloadSHA256: "old"}
	line := ListLine("wave1", e, "new")
	if !strings.Contains(line, "DRIFT") {
		t.Errorf("untracked drift must stay loud: %q", line)
	}
}

func TestListLineInSync(t *testing.T) {
	e := Entry{ProposalID: 142931, PayloadSHA256: "same", Status: StateExecuted}
	line := ListLine("wave1", e, "same")
	if !strings.Contains(line, "in sync") {
		t.Errorf("matching hash reads in sync: %q", line)
	}
	if !strings.Contains(line, "executed") {
		t.Errorf("state should still be shown: %q", line)
	}
}
