package nns

import "testing"

func TestApplyDecisionNotSubmitted(t *testing.T) {
	st := &State{Proposals: map[string]Entry{}}
	d, prev, err := st.ApplyDecision("x", "hash1", false)
	if err != nil {
		t.Fatal(err)
	}
	if d != ApplyProceed {
		t.Errorf("fresh proposal should proceed, got %v", d)
	}
	if prev != nil {
		t.Errorf("no prior entry expected, got %+v", prev)
	}
}

func TestApplyDecisionAlreadySubmittedSameHash(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, PayloadSHA256: "hash1"},
	}}
	d, prev, err := st.ApplyDecision("x", "hash1", false)
	if err != nil {
		t.Fatal(err)
	}
	if d != ApplyNothingToDo {
		t.Errorf("unchanged payload should be a no-op, got %v", d)
	}
	if prev == nil || prev.ProposalID != 42 {
		t.Errorf("verdict must carry the prior entry, got %+v", prev)
	}
}

func TestApplyDecisionDriftWithoutForce(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, PayloadSHA256: "old"},
	}}
	_, prev, err := st.ApplyDecision("x", "new", false)
	if err == nil {
		t.Fatal("drifted payload without --force must error")
	}
	if prev == nil || prev.ProposalID != 42 {
		t.Errorf("drift verdict must carry the prior entry, got %+v", prev)
	}
}

func TestApplyDecisionForceOverridesSameHash(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, PayloadSHA256: "hash1"},
	}}
	d, prev, err := st.ApplyDecision("x", "hash1", true)
	if err != nil {
		t.Fatal(err)
	}
	if d != ApplyProceed {
		t.Errorf("--force should proceed even when unchanged, got %v", d)
	}
	if prev == nil {
		t.Error("--force over a real prior submission should still report it")
	}
}

func TestApplyDecisionForceOverridesDrift(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 42, PayloadSHA256: "old"},
	}}
	d, _, err := st.ApplyDecision("x", "new", true)
	if err != nil {
		t.Fatal(err)
	}
	if d != ApplyProceed {
		t.Errorf("--force should override drift, got %v", d)
	}
}

// A recorded entry with a zero proposal id (e.g. never truly submitted) is not
// a real prior submission and must not block a fresh apply.
func TestApplyDecisionZeroProposalID(t *testing.T) {
	st := &State{Proposals: map[string]Entry{
		"x": {ProposalID: 0, PayloadSHA256: "hash1"},
	}}
	d, prev, err := st.ApplyDecision("x", "hash1", false)
	if err != nil {
		t.Fatal(err)
	}
	if d != ApplyProceed {
		t.Errorf("zero proposal id should proceed, got %v", d)
	}
	if prev != nil {
		t.Errorf("zero proposal id is not a real prior submission, got %+v", prev)
	}
}
