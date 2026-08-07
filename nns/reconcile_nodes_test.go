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

// A decommissioned node is expected to be gone: its deregistration is the declared
// state, not drift. Decommissioned blocks exist so historical proposal payloads keep
// resolving to the ids they were submitted with.
func TestReconcileOperatorNodesDecommissioned(t *testing.T) {
	r := &Resources{
		Operators: []NodeOperator{{Name: "op_a", ID: "op-a", Label: ""}},
		Nodes: []NodeRes{
			{Name: "n_new", ID: "n-new", Operator: "op-a"},
			{Name: "n_old", ID: "n-old", Operator: "op-a", Decommissioned: true},
		},
		labels: map[string]string{},
	}
	byOperator := map[string][]string{"op-a": {"n-new"}}

	nr := ReconcileOperatorNodes(r, byOperator)

	if got := nodeRowFor(nr, "n-old"); got == nil || got.Status != NodeOwnershipDecommissioned {
		t.Errorf("n-old: got %+v, want decommissioned", got)
	}
	if nr.HasDrift() {
		t.Errorf("a decommissioned node that is gone is not drift, got %+v", nr.Nodes)
	}
}

// A node declared decommissioned that the registry still says is owned is drift: the
// config claims it is decommissioned but it is not.
func TestReconcileOperatorNodesDecommissionedButStillOwned(t *testing.T) {
	r := &Resources{
		Operators: []NodeOperator{{Name: "op_a", ID: "op-a", Label: ""}},
		Nodes:     []NodeRes{{Name: "n_old", ID: "n-old", Operator: "op-a", Decommissioned: true}},
		labels:    map[string]string{},
	}
	byOperator := map[string][]string{"op-a": {"n-old"}}

	nr := ReconcileOperatorNodes(r, byOperator)

	if got := nodeRowFor(nr, "n-old"); got == nil || got.Status != NodeOwnershipMismatch {
		t.Errorf("n-old: got %+v, want mismatch", got)
	}
	if !nr.HasDrift() {
		t.Error("expected drift: declared decommissioned but still owned on-chain")
	}
}

// Absence from every declared operator's node list is not proof of
// deregistration: a node moved to an operator we do not declare looks identical.
// The node record settles it, so a decommissioned node the registry still
// registers is drift even when no declared operator owns it.
func TestReconcileOperatorNodesDecommissionedButStillRegistered(t *testing.T) {
	r := &Resources{
		Operators: []NodeOperator{{Name: "op_a", ID: "op-a", Label: ""}},
		Nodes:     []NodeRes{{Name: "n_old", ID: "n-old", Operator: "op-a", Decommissioned: true}},
		labels:    map[string]string{},
	}
	// No declared operator owns it, but the registry still has a live record.
	nr := ReconcileOperatorNodes(r, map[string][]string{"op-a": {}})
	nr.ApplyNodeStatus(map[string]NodeStatus{"n-old": {Registered: true, OperatorID: "undeclared-op"}})

	got := nodeRowFor(nr, "n-old")
	if got == nil || got.Status != NodeOwnershipMismatch {
		t.Fatalf("n-old: got %+v, want mismatch", got)
	}
	if !nr.HasDrift() {
		t.Error("a decommissioned node still in the registry is drift")
	}
}

// The confirming case: the record is gone, so decommissioned holds.
func TestReconcileOperatorNodesDecommissionedConfirmed(t *testing.T) {
	r := &Resources{
		Operators: []NodeOperator{{Name: "op_a", ID: "op-a", Label: ""}},
		Nodes:     []NodeRes{{Name: "n_old", ID: "n-old", Operator: "op-a", Decommissioned: true}},
		labels:    map[string]string{},
	}
	nr := ReconcileOperatorNodes(r, map[string][]string{"op-a": {}})
	nr.ApplyNodeStatus(map[string]NodeStatus{"n-old": {Registered: false}})

	if got := nodeRowFor(nr, "n-old"); got == nil || got.Status != NodeOwnershipDecommissioned {
		t.Errorf("n-old: got %+v, want decommissioned", got)
	}
	if nr.HasDrift() {
		t.Error("a confirmed decommissioned node is not drift")
	}
}

// Without a record the verdict stands as before: absence from declared
// operators is the only evidence available, and it is not worth failing on.
func TestReconcileOperatorNodesDecommissionedUnchecked(t *testing.T) {
	r := &Resources{
		Operators: []NodeOperator{{Name: "op_a", ID: "op-a", Label: ""}},
		Nodes:     []NodeRes{{Name: "n_old", ID: "n-old", Operator: "op-a", Decommissioned: true}},
		labels:    map[string]string{},
	}
	nr := ReconcileOperatorNodes(r, map[string][]string{"op-a": {}})
	nr.ApplyNodeStatus(map[string]NodeStatus{})

	if got := nodeRowFor(nr, "n-old"); got == nil || got.Status != NodeOwnershipDecommissioned {
		t.Errorf("n-old: got %+v, want decommissioned", got)
	}
	if nr.HasDrift() {
		t.Error("an unread record is not drift")
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
