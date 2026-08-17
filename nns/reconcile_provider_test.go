package nns

import (
	"strings"
	"testing"
)

func opRowFor(pr ProviderReconcile, opID string) *OperatorRow {
	for i := range pr.Operators {
		if pr.Operators[i].OperatorID == opID {
			return &pr.Operators[i]
		}
	}
	return nil
}

func TestReconcileProviders(t *testing.T) {
	r := &Resources{
		Providers: []NodeProvider{
			{Name: "alpinedc", ID: "prov-a", Label: "AlpineDC"},
			{Name: "ghost", ID: "prov-x", Label: "Ghost"},
		},
		DCs: []DataCenter{
			{Name: "vd1", ID: "vd1", Label: "", Region: "Europe,LI,Vaduz"},
		},
		Operators: []NodeOperator{
			{Name: "a_ok", ID: "op-ok", Label: "", Provider: "prov-a", Dc: "vd1"},   // matches canister
			{Name: "a_dc", ID: "op-dc", Label: "", Provider: "prov-a", Dc: "so1"},   // dc mismatch
			{Name: "a_unk", ID: "op-unk", Label: "", Provider: "prov-a", Dc: "vd1"}, // provider does not own it
			{Name: "x_op", ID: "op-x", Label: "", Provider: "prov-x", Dc: "vd1"},    // provider owns nothing
		},
		labels: map[string]string{},
	}
	// Canister truth: prov-a owns op-ok (vd1) and op-dc (vd1); prov-x returns nothing.
	byProvider := map[string][]ProviderOperator{
		"prov-a": {
			{OperatorID: "op-ok", DcID: "vd1", DcRegion: "Europe,LI,Vaduz"},
			{OperatorID: "op-dc", DcID: "vd1", DcRegion: "Europe,LI,Vaduz"},
		},
		"prov-x": nil,
	}

	pr := ReconcileProviders(r, byProvider)

	if got := opRowFor(pr, "op-ok"); got == nil || got.Status != OperatorOK {
		t.Errorf("op-ok: got %+v, want ok", got)
	}
	if got := opRowFor(pr, "op-dc"); got == nil || got.Status != OperatorDcMismatch {
		t.Errorf("op-dc: got %+v, want dc-mismatch", got)
	}
	if got := opRowFor(pr, "op-unk"); got == nil || got.Status != OperatorUnknown {
		t.Errorf("op-unk: got %+v, want unknown", got)
	}
	// prov-x was queried and returned nothing: op-x is not owned. The registry
	// cannot say whether prov-x itself exists.
	if got := opRowFor(pr, "op-x"); got == nil || got.Status != OperatorUnknown {
		t.Errorf("op-x: got %+v, want unknown", got)
	}
	if !pr.HasDrift() {
		t.Error("expected drift")
	}
}

func TestReconcileProvidersEmptyProviderIsNotAbsent(t *testing.T) {
	// The registry answers Ok{} both for a provider that owns no operators and
	// for a principal that is no provider at all, so an empty result cannot claim
	// the provider is absent. The operator is simply not owned.
	r := &Resources{
		Providers: []NodeProvider{{Name: "empty", ID: "prov-empty"}},
		Operators: []NodeOperator{{Name: "orphan", ID: "op-orphan", Provider: "prov-empty"}},
		labels:    map[string]string{},
	}
	pr := ReconcileProviders(r, map[string][]ProviderOperator{"prov-empty": {}})

	got := opRowFor(pr, "op-orphan")
	if got == nil || got.Status != OperatorUnknown {
		t.Fatalf("op-orphan: got %+v, want unknown", got)
	}
	if strings.Contains(got.Detail, "provider not found") {
		t.Errorf("detail claims the provider is absent, which the registry cannot prove: %q", got.Detail)
	}
	if !pr.HasDrift() {
		t.Error("an unowned operator is still drift")
	}
}

func TestReconcileProvidersUnqueriedProvider(t *testing.T) {
	// A provider missing from byProvider was never queried: report that, rather
	// than implying the registry denied it.
	r := &Resources{
		Providers: []NodeProvider{{Name: "p", ID: "prov-p"}},
		Operators: []NodeOperator{{Name: "op", ID: "op-1", Provider: "prov-p"}},
		labels:    map[string]string{},
	}
	pr := ReconcileProviders(r, map[string][]ProviderOperator{})

	got := opRowFor(pr, "op-1")
	if got == nil || got.Status != OperatorProviderUnchecked {
		t.Fatalf("op-1: got %+v, want provider-unchecked", got)
	}
}

func TestReconcileProvidersDecommissioned(t *testing.T) {
	// A decommissioned operator or provider is expected to be absent on-chain;
	// still being there is the drift.
	r := &Resources{
		Providers: []NodeProvider{
			{Name: "gone_prov", ID: "prov-gone", Decommissioned: true},
			{Name: "live_prov", ID: "prov-live"},
		},
		Operators: []NodeOperator{
			{Name: "op_gone", ID: "op-gone", Provider: "prov-live", Decommissioned: true},
			{Name: "op_lingers", ID: "op-lingers", Provider: "prov-live", Decommissioned: true},
			{Name: "under_gone_prov", ID: "op-under", Provider: "prov-gone"},
			{Name: "op_live", ID: "op-live", Provider: "prov-live"},
		},
		labels: map[string]string{},
	}
	byProvider := map[string][]ProviderOperator{
		"prov-gone": nil,
		"prov-live": {
			{OperatorID: "op-live"},
			{OperatorID: "op-lingers"}, // declared gone, still owned
		},
	}

	pr := ReconcileProviders(r, byProvider)

	// Absent as declared: not drift.
	if got := opRowFor(pr, "op-gone"); got == nil || got.Status != OperatorDecommissioned {
		t.Errorf("op-gone: got %+v, want decommissioned", got)
	}
	// An operator under a decommissioned provider is expected to be gone too.
	if got := opRowFor(pr, "op-under"); got == nil || got.Status != OperatorDecommissioned {
		t.Errorf("op-under: got %+v, want decommissioned", got)
	}
	// Declared gone but the registry still lists it: drift.
	if got := opRowFor(pr, "op-lingers"); got == nil || got.Status != OperatorStillRegistered {
		t.Errorf("op-lingers: got %+v, want still-registered", got)
	}
	if got := opRowFor(pr, "op-live"); got == nil || got.Status != OperatorOK {
		t.Errorf("op-live: got %+v, want ok", got)
	}
	if !pr.HasDrift() {
		t.Error("expected drift from the lingering operator")
	}
}

func TestReconcileProvidersDecommissionedClean(t *testing.T) {
	// Everything decommissioned and absent: no drift at all.
	r := &Resources{
		Providers: []NodeProvider{{Name: "gone_prov", ID: "prov-gone", Decommissioned: true}},
		Operators: []NodeOperator{
			{Name: "op_a", ID: "op-a", Provider: "prov-gone", Decommissioned: true},
			{Name: "op_b", ID: "op-b", Provider: "prov-gone"},
		},
		labels: map[string]string{},
	}
	pr := ReconcileProviders(r, map[string][]ProviderOperator{"prov-gone": nil})
	if pr.HasDrift() {
		t.Errorf("decommissioned provider reported drift: %+v", pr.Operators)
	}
}

func TestReconcileProvidersClean(t *testing.T) {
	r := &Resources{
		Providers: []NodeProvider{{Name: "p", ID: "prov", Label: ""}},
		Operators: []NodeOperator{{Name: "o", ID: "op", Label: "", Provider: "prov", Dc: "vd1"}},
		labels:    map[string]string{},
	}
	byProvider := map[string][]ProviderOperator{
		"prov": {{OperatorID: "op", DcID: "vd1"}},
	}
	pr := ReconcileProviders(r, byProvider)
	if pr.HasDrift() {
		t.Errorf("expected no drift, got %+v", pr.Operators)
	}
}
