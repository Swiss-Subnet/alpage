package nns

import (
	"strings"
	"testing"
)

func dcResources() *Resources {
	return &Resources{DCs: []DataCenter{
		{Name: "zh3", ID: "zh3", Region: "Europe,CH,Zurich"},
		{Name: "gva1", ID: "gva1", Region: "Europe,CH,Geneva"},
	}}
}

// A data center with no operator under it was previously unchecked, since the
// only region comparison ran through an operator's provider.
func TestReconcileDataCentersFlagsMissingRecord(t *testing.T) {
	dr := ReconcileDataCenters(dcResources(), map[string]DataCenterSpec{
		"zh3": {ID: "zh3", Region: "Europe,CH,Zurich"},
	})
	if !dr.HasDrift() {
		t.Fatal("a declared data center absent from the registry is drift")
	}
	var row DataCenterRow
	for _, r := range dr.DataCenters {
		if r.DcID == "gva1" {
			row = r
		}
	}
	if row.Status != DataCenterMissing {
		t.Fatalf("gva1 status = %q, want missing", row.Status)
	}
	// Drift the operator can act on has to name the proposal that resolves it.
	if got := remedyOf(row.Status); !strings.Contains(got, "add_data_centers") {
		t.Errorf("remedy should name the proposal that fixes it, got %q", got)
	}
}

func TestReconcileDataCentersFlagsRegionMismatch(t *testing.T) {
	dr := ReconcileDataCenters(dcResources(), map[string]DataCenterSpec{
		"zh3":  {ID: "zh3", Region: "Europe,CH,Geneva"},
		"gva1": {ID: "gva1", Region: "Europe,CH,Geneva"},
	})
	if !dr.HasDrift() {
		t.Fatal("a region that differs on-chain is drift")
	}
	if got := dr.DataCenters[1].Detail; !strings.Contains(got, "Zurich") || !strings.Contains(got, "Geneva") {
		t.Errorf("detail should show both regions, got %q", got)
	}
}

func TestReconcileDataCentersCleanWhenMatching(t *testing.T) {
	dr := ReconcileDataCenters(dcResources(), map[string]DataCenterSpec{
		"zh3":  {ID: "zh3", Region: "Europe,CH,Zurich"},
		"gva1": {ID: "gva1", Region: "Europe,CH,Geneva"},
	})
	if dr.HasDrift() {
		t.Errorf("matching records are not drift: %+v", dr.DataCenters)
	}
}

// A block declaring no region asserts nothing about it, so the record's region
// is accepted rather than reported.
func TestReconcileDataCentersRegionUncheckedIsNotDrift(t *testing.T) {
	r := &Resources{DCs: []DataCenter{{Name: "zh3", ID: "zh3"}}}
	dr := ReconcileDataCenters(r, map[string]DataCenterSpec{
		"zh3": {ID: "zh3", Region: "Europe,CH,Zurich"},
	})
	if dr.HasDrift() {
		t.Error("an undeclared region is not drift")
	}
	if dr.DataCenters[0].Status != DataCenterRegionUnchecked {
		t.Errorf("status = %q, want region-unchecked", dr.DataCenters[0].Status)
	}
}
