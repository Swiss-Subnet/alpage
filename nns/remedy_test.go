package nns

import "testing"

// Every status each reconciler can classify into, paired with whether it counts
// as drift. The OK flags mirror what each Table builder passes, so a status
// that changes sides here without the builder changing too is a bug in one of
// them.
var allReconcileStatuses = []struct {
	status Remedier
	name   string
	drift  bool
}{
	{ReconcileInSync, "in-sync", false},
	{ReconcileMissing, "missing", true},
	{ReconcileUnmanaged, "unmanaged", true},
	{ReconcileDeregistered, "deregistered", true},

	{DataCenterOK, "dc ok", false},
	{DataCenterRegionUnchecked, "dc region-unchecked", false},
	{DataCenterMissing, "dc missing", true},
	{DataCenterRegionMismatch, "dc region-mismatch", true},

	{OperatorOK, "operator ok", false},
	{OperatorDecommissioned, "operator decommissioned", false},
	{OperatorDcMismatch, "operator dc-mismatch", true},
	{OperatorUnknown, "operator unknown", true},
	{OperatorProviderUnchecked, "operator provider-unchecked", true},
	{OperatorStillRegistered, "operator still-registered", true},

	{NodeSevInSync, "sev in-sync", false},
	{NodeSevUnknown, "sev unknown", false},
	{NodeSevMismatch, "sev chip-changed", true},
	{NodeSevMissing, "sev chip-missing", true},
	{NodeSevUndeclared, "sev chip-undeclared", true},
	{NodeSevMalformed, "sev malformed", true},
	{NodeSevUnverified, "sev amd-unverified", true},
}

// A row that says what is wrong without saying what to do sends the reader
// hunting for the proposal kind that fixes it. Every drift status has to carry
// one, and a status that is not drift must not, since advice beside an in-sync
// row reads as a problem.
func TestEveryDriftStatusHasRemedy(t *testing.T) {
	for _, tc := range allReconcileStatuses {
		got := remedyOf(tc.status)
		switch {
		case tc.drift && got == "":
			t.Errorf("%s is drift but names no remedy", tc.name)
		case !tc.drift && got != "":
			t.Errorf("%s is not drift but names remedy %q", tc.name, got)
		}
	}
}

// The two row-level remediers branch on more than the status, so they are
// checked through the row rather than through the enum.
func TestRowLevelRemedies(t *testing.T) {
	live := NodeOwnershipRow{Status: NodeOwnershipMismatch, Operator: "op-1", Owner: "op-2"}
	if remedyOf(live) == "" {
		t.Error("an operator mismatch names no remedy")
	}
	retired := NodeOwnershipRow{Status: NodeOwnershipMismatch, Decommissioned: true}
	if remedyOf(live) == remedyOf(retired) {
		t.Error("a decommissioned node that is still owned needs different advice from a live mismatch")
	}
	if remedyOf(NodeOwnershipRow{Status: NodeOwnershipOK}) != "" {
		t.Error("an in-sync node must name no remedy")
	}

	unelected := NodeVersionRow{Status: NodeVersionMismatch, Unelected: true}
	elected := NodeVersionRow{Status: NodeVersionMismatch}
	if remedyOf(unelected) == remedyOf(elected) {
		t.Error("an unelected declaration cannot be fixed by a deploy, so it needs its own advice")
	}
	if remedyOf(NodeVersionRow{Status: NodeVersionOK}) != "" {
		t.Error("a matching version must name no remedy")
	}
}

// The remedy is for acting on drift, so an in-sync row drops it even when one
// is set.
func TestRenderShowsRemedyOnlyForDrift(t *testing.T) {
	drift := Row{Status: "missing", ID: "n1", Remedy: "do the thing"}
	if got := rowRemedy(drift); got == "" {
		t.Error("a drift row must render its remedy")
	}
	fine := Row{Status: "in-sync", ID: "n1", Remedy: "do the thing", OK: true}
	if got := rowRemedy(fine); got != "" {
		t.Errorf("an in-sync row must not render a remedy, got %q", got)
	}
}
