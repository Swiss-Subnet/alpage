package nns

import "testing"

func nodeRowFor(nr NodeOwnershipReconcile, id string) *NodeOwnershipRow {
	for i := range nr.Nodes {
		if nr.Nodes[i].NodeID == id {
			return &nr.Nodes[i]
		}
	}
	return nil
}

func TestReconcileOperatorNodes(t *testing.T) {
	r := &Resources{
		Operators: []NodeOperator{
			{Name: "op_a", ID: "op-a", Label: ""},
			{Name: "op_b", ID: "op-b", Label: ""},
		},
		Nodes: []NodeRes{
			{Name: "n_ok", ID: "n-ok", Label: "", Operator: "op-a"},       // op-a owns it: ok
			{Name: "n_moved", ID: "n-moved", Label: "", Operator: "op-a"}, // op-b owns it now: mismatch
			{Name: "n_gone", ID: "n-gone", Label: "", Operator: "op-a"},   // no operator owns it: gone
		},
		labels: map[string]string{},
	}
	// Canister truth: op-a owns n-ok; op-b owns n-moved and n-extra (undeclared).
	byOperator := map[string][]string{
		"op-a": {"n-ok"},
		"op-b": {"n-moved", "n-extra"},
	}

	nr := ReconcileOperatorNodes(r, byOperator)

	if got := nodeRowFor(nr, "n-ok"); got == nil || got.Status != NodeOwnershipOK {
		t.Errorf("n-ok: got %+v, want ok", got)
	}
	if got := nodeRowFor(nr, "n-moved"); got == nil || got.Status != NodeOwnershipMismatch {
		t.Errorf("n-moved: got %+v, want mismatch", got)
	}
	if got := nodeRowFor(nr, "n-gone"); got == nil || got.Status != NodeOwnershipGone {
		t.Errorf("n-gone: got %+v, want gone", got)
	}
	if got := nodeRowFor(nr, "n-extra"); got == nil || got.Status != NodeOwnershipUnmanaged {
		t.Errorf("n-extra: got %+v, want unmanaged", got)
	}
	if !nr.HasDrift() {
		t.Error("expected drift")
	}
}

func TestReconcileOperatorNodesClean(t *testing.T) {
	r := &Resources{
		Operators: []NodeOperator{{Name: "op_a", ID: "op-a", Label: ""}},
		Nodes:     []NodeRes{{Name: "n", ID: "n-1", Label: "", Operator: "op-a"}},
		labels:    map[string]string{},
	}
	byOperator := map[string][]string{"op-a": {"n-1"}}
	nr := ReconcileOperatorNodes(r, byOperator)
	if nr.HasDrift() {
		t.Errorf("expected no drift, got %+v", nr.Nodes)
	}
}

// A node that declares no operator is not diffed against ownership at all.
func TestReconcileOperatorNodesSkipsUndeclaredOperator(t *testing.T) {
	r := &Resources{
		Operators: []NodeOperator{{Name: "op_a", ID: "op-a", Label: ""}},
		Nodes:     []NodeRes{{Name: "n", ID: "n-1", Label: ""}}, // no Operator field
		labels:    map[string]string{},
	}
	byOperator := map[string][]string{"op-a": {"n-1"}}
	nr := ReconcileOperatorNodes(r, byOperator)
	// n-1 is owned by op-a on-chain but never declared with an operator: it is
	// reported as unmanaged, not silently ignored.
	if got := nodeRowFor(nr, "n-1"); got == nil || got.Status != NodeOwnershipUnmanaged {
		t.Errorf("n-1: got %+v, want unmanaged", got)
	}
}
