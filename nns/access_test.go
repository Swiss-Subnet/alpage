package nns

import (
	"os"
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
	"github.com/swiss-subnet/alpage/pocketic"
)

func TestClassifyAccessUnknownIsNone(t *testing.T) {
	caller := principal.MustDecode("aaaaa-aa")
	other := principal.MustDecode("2vxsx-fae")
	n := &governance.Neuron{Controller: &other} // caller is neither controller nor hotkey
	if got := classifyAccess(caller, n); got != NeuronAccessNone {
		t.Errorf("classifyAccess = %v, want none", got)
	}
}

func TestClassifyAccess(t *testing.T) {
	ctrl := principal.MustDecode("rrkah-fqaaa-aaaaa-aaaaq-cai")
	hk := principal.MustDecode("rwlgt-iiaaa-aaaaa-aaaaa-cai")
	stranger := principal.MustDecode("r7inp-6aaaa-aaaaa-aaabq-cai")

	tests := []struct {
		name   string
		caller principal.Principal
		neuron *governance.Neuron
		want   NeuronAccess
	}{
		{"controller", ctrl, &governance.Neuron{Controller: &ctrl, HotKeys: []principal.Principal{hk}}, NeuronAccessController},
		{"hotkey", hk, &governance.Neuron{Controller: &ctrl, HotKeys: []principal.Principal{hk}}, NeuronAccessHotkey},
		{"unclassified grants nothing", stranger, &governance.Neuron{Controller: &ctrl, HotKeys: []principal.Principal{hk}}, NeuronAccessNone},
		{"nil controller no hotkeys", stranger, &governance.Neuron{}, NeuronAccessNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyAccess(tt.caller, tt.neuron); got != tt.want {
				t.Errorf("classifyAccess = %v, want %v", got, tt.want)
			}
		})
	}
}

// startNNSWithIdentities boots a live NNS with controllerID controlling the
// proposer neuron and hotkeyID registered, returning the gateway URL.
func startNNSWithIdentities(t *testing.T, controllerID, hotkeyID *Identity) string {
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
	if _, err := BringUpWithHotkey(c, inst, controllerID.Principal(), hotkeyID.Principal()); err != nil {
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
	url, err := c.MakeLive(inst)
	if err != nil {
		t.Fatalf("make live: %v", err)
	}
	t.Cleanup(func() { _ = c.StopGateway(inst) })
	return url
}

func mustIdentity(t *testing.T) *Identity {
	t.Helper()
	id, err := NewIdentity()
	if err != nil {
		t.Fatalf("new identity: %v", err)
	}
	return id
}

func TestCheckNeuronAccessHotkey(t *testing.T) {
	controllerID, hotkeyID := mustIdentity(t), mustIdentity(t)
	url := startNNSWithIdentities(t, controllerID, hotkeyID)

	got, err := CheckNeuronAccess(hotkeyID, url, true, ProposerNeuronID, DisableQueryVerification())
	if err != nil {
		t.Fatalf("CheckNeuronAccess: %v", err)
	}
	if got != NeuronAccessHotkey {
		t.Errorf("access = %v, want hotkey", got)
	}
}

func TestCheckNeuronAccessController(t *testing.T) {
	controllerID, hotkeyID := mustIdentity(t), mustIdentity(t)
	url := startNNSWithIdentities(t, controllerID, hotkeyID)

	got, err := CheckNeuronAccess(controllerID, url, true, ProposerNeuronID, DisableQueryVerification())
	if err != nil {
		t.Fatalf("CheckNeuronAccess: %v", err)
	}
	if got != NeuronAccessController {
		t.Errorf("access = %v, want controller", got)
	}
}

func TestCheckNeuronAccessDenied(t *testing.T) {
	controllerID, hotkeyID := mustIdentity(t), mustIdentity(t)
	url := startNNSWithIdentities(t, controllerID, hotkeyID)

	stranger := mustIdentity(t)
	got, err := CheckNeuronAccess(stranger, url, true, ProposerNeuronID, DisableQueryVerification())
	if err == nil {
		t.Fatalf("expected access denied, got %v", got)
	}
	if got != NeuronAccessNone {
		t.Errorf("access = %v, want none", got)
	}
	if !strings.Contains(err.Error(), "cannot access neuron") {
		t.Errorf("error = %q, want it to mention lack of access", err)
	}
}
