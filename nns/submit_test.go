package nns

import (
	"errors"
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
)

type fakeSubmitter struct {
	pid    uint64
	err    error
	calls  int
	gotAct Action
}

func (f *fakeSubmitter) Submit(neuron governance.NeuronId, a Action) (uint64, error) {
	f.calls++
	f.gotAct = a
	return f.pid, f.err
}

func recordArgs() RecordArgs {
	return RecordArgs{
		Name:        "x",
		Kind:        "membership",
		Hash:        "hash1",
		SubmittedBy: "prin",
		Neuron:      7,
		Host:        MainnetHost,
		At:          "2026-07-22T00:00:00Z",
	}
}

var fakeAction = DeployGuestosAction{SubnetID: principal.Principal{Raw: []byte{1}}, ReplicaVersionID: "v"}

func TestSubmitAndRecordSuccess(t *testing.T) {
	st := &State{Proposals: map[string]Entry{}}
	sub := &fakeSubmitter{pid: 999}
	saved := 0
	save := func(s *State) error { saved++; return nil }

	pid, err := SubmitAndRecord(sub, st, governance.NeuronId{Id: 7}, fakeAction, recordArgs(), save)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 999 {
		t.Errorf("pid = %d, want 999", pid)
	}
	if sub.calls != 1 || sub.gotAct.Kind() != "deploy_guestos" {
		t.Errorf("submitter called %d times with %v", sub.calls, sub.gotAct)
	}
	if saved != 1 {
		t.Errorf("state saved %d times, want 1", saved)
	}
	e := st.Proposals["x"]
	if e.ProposalID != 999 || e.PayloadSHA256 != "hash1" || e.Kind != "membership" || e.SubmittedBy != "prin" || e.Neuron != 7 {
		t.Errorf("recorded entry wrong: %+v", e)
	}
}

func TestSubmitAndRecordSubmitFailsWritesNothing(t *testing.T) {
	st := &State{Proposals: map[string]Entry{}}
	sub := &fakeSubmitter{err: errors.New("governance rejected")}
	saved := 0
	save := func(s *State) error { saved++; return nil }

	_, err := SubmitAndRecord(sub, st, governance.NeuronId{Id: 7}, fakeAction, recordArgs(), save)
	if err == nil {
		t.Fatal("expected submit error to propagate")
	}
	if saved != 0 {
		t.Errorf("state must not be written when submit fails, saved=%d", saved)
	}
	if _, ok := st.Proposals["x"]; ok {
		t.Error("no entry should be recorded when submit fails")
	}
}

// The worst case: the proposal was submitted (has a real id) but the state
// write failed. The error must surface the id so the operator can recover it.
func TestSubmitAndRecordStateWriteFails(t *testing.T) {
	st := &State{Proposals: map[string]Entry{}}
	sub := &fakeSubmitter{pid: 12345}
	save := func(s *State) error { return errors.New("disk full") }

	pid, err := SubmitAndRecord(sub, st, governance.NeuronId{Id: 7}, fakeAction, recordArgs(), save)
	if err == nil {
		t.Fatal("expected state-write error")
	}
	if pid != 12345 {
		t.Errorf("pid should still be returned so the caller can report it, got %d", pid)
	}
	if !strings.Contains(err.Error(), "12345") {
		t.Errorf("error must mention the submitted proposal id, got %q", err)
	}
}
