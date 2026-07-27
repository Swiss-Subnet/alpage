package nns

import "testing"

// The dry-run is local and must run before anything touches mainnet, so a
// caller with no neuron access still gets the payload verified. The two network
// gates keep their fail-fast ordering relative to each other, ahead of any
// submission.
func TestApplyPhaseOrder(t *testing.T) {
	got := ApplyPhases()
	want := []ApplyPhase{PhaseDryRun, PhaseNeuronAccess, PhasePlan}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("phase %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

func TestApplyPhaseDryRunIsLocal(t *testing.T) {
	if PhaseDryRun.NeedsNetwork() {
		t.Error("the dry-run must not require mainnet")
	}
	for _, p := range []ApplyPhase{PhaseNeuronAccess, PhasePlan} {
		if !p.NeedsNetwork() {
			t.Errorf("%s queries mainnet", p)
		}
	}
}
