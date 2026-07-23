package nns

import "testing"

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
		Providers: []Resource{
			{Name: "alpinedc", ID: "prov-a", Label: "AlpineDC"},
			{Name: "ghost", ID: "prov-x", Label: "Ghost"},
		},
		DCs: []Resource{
			{Name: "vd1", ID: "vd1", Region: "Europe,LI,Vaduz"},
		},
		Operators: []Resource{
			{Name: "a_ok", ID: "op-ok", Provider: "prov-a", Dc: "vd1"},   // matches canister
			{Name: "a_dc", ID: "op-dc", Provider: "prov-a", Dc: "so1"},   // dc mismatch
			{Name: "a_unk", ID: "op-unk", Provider: "prov-a", Dc: "vd1"}, // provider does not own it
			{Name: "x_op", ID: "op-x", Provider: "prov-x", Dc: "vd1"},    // provider absent
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
	if got := opRowFor(pr, "op-x"); got == nil || got.Status != OperatorProviderAbsent {
		t.Errorf("op-x: got %+v, want provider-absent", got)
	}
	if !pr.HasDrift() {
		t.Error("expected drift")
	}
}

func TestReconcileProvidersClean(t *testing.T) {
	r := &Resources{
		Providers: []Resource{{Name: "p", ID: "prov"}},
		Operators: []Resource{{Name: "o", ID: "op", Provider: "prov", Dc: "vd1"}},
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
