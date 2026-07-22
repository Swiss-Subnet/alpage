package nns

import (
	"os"
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/pocketic"
)

// startNNS boots a PocketIC instance, brings up the NNS with the given hotkey
// (empty for none), and advances the clock so governance seeds its RNG and the
// proposer neuron has voting power.
func startNNS(t *testing.T, hotkey principal.Principal) *NNS {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping PocketIC-backed test in -short mode")
	}
	if os.Getenv("POCKET_IC_BIN") == "" {
		t.Skip("POCKET_IC_BIN not set; run inside nix develop")
	}
	c, err := pocketic.Start("")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	inst, err := c.NewInstance()
	if err != nil {
		t.Fatalf("instance: %v", err)
	}
	n, err := BringUpWithHotkey(c, inst, testController(), hotkey)
	if err != nil {
		t.Fatalf("bring up: %v", err)
	}
	if err := c.SetTime(inst, 1_700_000_000_000_000_000); err != nil {
		t.Fatalf("set_time: %v", err)
	}
	if err := c.AutoProgress(inst); err != nil {
		t.Fatalf("auto_progress: %v", err)
	}
	for i := 0; i < 5; i++ {
		_ = c.Tick(inst)
	}
	return n
}

func assertResizeRendered(t *testing.T, rendered string) {
	t.Helper()
	for _, want := range []string{
		"change_subnet_membership",
		"3zsyy-cnoqf",
		"nodes removed (6)",
		"nodes added (0)",
		"ezsx4-peoff", // spot-check two of the six removed node ids
		"vou34-3jw7y",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered proposal missing %q", want)
		}
	}
}

func TestSwissSubnetWave1(t *testing.T) {
	n := startNNS(t, principal.Principal{})
	neuron := n.ProposerNeuron()

	pid, err := n.SubmitResize(neuron, SwissSubnetWave1)
	if err != nil {
		t.Fatalf("submit resize: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	rendered := Render(pi)
	t.Logf("\n%s", rendered)

	assertResizeRendered(t, rendered)
	// The seeded neuron has voting power, so the proposal is adopted (it then
	// fails at execution because the local registry has no subnet records,
	// which is expected for a formatting harness).
	if pi.LatestTally == nil || pi.LatestTally.Yes == 0 {
		t.Errorf("expected a non-zero yes tally (adopted), got %+v", pi.LatestTally)
	}
}

func TestSubmitViaHotkey(t *testing.T) {
	hotkeyID, err := NewIdentity()
	if err != nil {
		t.Fatalf("generate hotkey identity: %v", err)
	}
	n := startNNS(t, hotkeyID.Principal())
	if len(n.Hotkey.Raw) == 0 {
		t.Fatal("hotkey not set on NNS")
	}

	// Submit as the hotkey principal, not the controller.
	pid, err := n.SubmitResizeAs(n.Hotkey, n.ProposerNeuron(), SwissSubnetWave1)
	if err != nil {
		t.Fatalf("submit via hotkey: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	rendered := Render(pi)
	t.Logf("\n%s", rendered)
	assertResizeRendered(t, rendered)
}
