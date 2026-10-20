package nns

import (
	"strings"
	"testing"
)

// do_remove_nodes panics on a node that is still a subnet member, and that
// happens after the NNS has adopted the proposal. Preflight exists to catch it
// while it is still a config error.
func TestPlanRemoveNodesBlocksSubnetMember(t *testing.T) {
	p := planRemoveNodes(
		[]string{"n-free", "n-member"},
		map[string]string{"n-member": "subnet-1"},
		map[string]bool{"n-free": true, "n-member": true},
	)
	if !p.HasBlocking() {
		t.Fatal("a node still in a subnet must block")
	}
	if len(p.Issues) != 1 || p.Issues[0].Target != "n-member" {
		t.Errorf("expected one issue for n-member, got %+v", p.Issues)
	}
	if preconditionPreflight(p).Level != PreflightWarn {
		t.Error("a blocking issue must surface as a warning")
	}
}

// An unregistered node is skipped by the canister rather than failing, so it is
// reported without blocking.
func TestPlanRemoveNodesUnknownIsNotBlocking(t *testing.T) {
	p := planRemoveNodes(
		[]string{"n-gone"},
		map[string]string{},
		map[string]bool{},
	)
	if p.HasBlocking() {
		t.Error("an already-absent node is not a blocking problem")
	}
	if !p.NoOp {
		t.Error("removing only absent nodes changes nothing")
	}
	if preconditionPreflight(p).Level != PreflightNoOp {
		t.Error("an all-absent removal is a no-op")
	}
}

func TestPlanRemoveNodesClean(t *testing.T) {
	p := planRemoveNodes(
		[]string{"n-1", "n-2"},
		map[string]string{},
		map[string]bool{"n-1": true, "n-2": true},
	)
	if len(p.Issues) != 0 {
		t.Errorf("expected no issues, got %+v", p.Issues)
	}
	pf := preconditionPreflight(p)
	if pf.Level != PreflightClean {
		t.Error("unassigned registered nodes remove cleanly")
	}
	// An issue-free removal still has to name its targets, so a clean plan never
	// prints nothing at all.
	if pf.Report == "" {
		t.Fatal("a clean removal must still report its targets")
	}
	for _, want := range []string{"n-1", "n-2", "remove_nodes"} {
		if !strings.Contains(pf.Report, want) {
			t.Errorf("clean report missing %q, got:\n%s", want, pf.Report)
		}
	}
}

// do_remove_node_operators silently filters out operators that still own nodes:
// the proposal executes, reports success, and the operator remains. Nothing
// surfaces that afterwards, so preflight is the only place to catch it.
func TestPlanRemoveNodeOperatorsBlocksOwnedNodes(t *testing.T) {
	p := planRemoveNodeOperators(
		[]string{"op-empty", "op-owns"},
		map[string][]string{"op-owns": {"n-1", "n-2"}},
		map[string]bool{"op-empty": true, "op-owns": true},
	)
	if !p.HasBlocking() {
		t.Fatal("an operator that still owns nodes must block")
	}
	if len(p.Issues) != 1 || p.Issues[0].Target != "op-owns" {
		t.Errorf("expected one issue for op-owns, got %+v", p.Issues)
	}
}

func TestPlanRemoveNodeOperatorsClean(t *testing.T) {
	p := planRemoveNodeOperators(
		[]string{"op-empty"},
		map[string][]string{},
		map[string]bool{"op-empty": true},
	)
	if len(p.Issues) != 0 {
		t.Errorf("expected no issues, got %+v", p.Issues)
	}
}

// Governance validates the provider exists before the vote, so an unregistered
// one is rejected outright rather than failing later.
func TestPlanRemoveNodeProviderBlocksUnregistered(t *testing.T) {
	p := planRemoveNodeProvider("np-1", false, nil)
	if !p.HasBlocking() {
		t.Fatal("an unregistered provider must block")
	}
}

// Removing a provider that still has operators orphans them, so it blocks: the
// three mainnet proposals ran nodes, then operators, then provider.
func TestPlanRemoveNodeProviderBlocksRemainingOperators(t *testing.T) {
	p := planRemoveNodeProvider("np-1", true, []string{"op-b", "op-a"})
	if !p.HasBlocking() {
		t.Fatal("a provider with operators still attached must block")
	}
	// Operators are listed sorted so the report does not churn between runs.
	if got := p.Issues[0].Detail; !strings.Contains(got, "op-a, op-b") {
		t.Errorf("operators should be listed sorted, got %q", got)
	}
}

func TestPlanRemoveNodeProviderClean(t *testing.T) {
	p := planRemoveNodeProvider("np-1", true, nil)
	if len(p.Issues) != 0 {
		t.Errorf("expected no issues, got %+v", p.Issues)
	}
}
