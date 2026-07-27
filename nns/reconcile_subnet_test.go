package nns

import "testing"

const testSubnetID = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"

// Absent sev_enabled asserts not-enabled rather than skipping the check, so an
// undeclared subnet that is SEV-enabled on-chain is drift.
func TestReconcileSubnetFeaturesAbsentAssertsDisabled(t *testing.T) {
	sn := Subnet{Name: "swiss", ID: testSubnetID}
	fr := ReconcileSubnetFeatures(sn, SubnetFeatures{SevEnabled: new(true)})
	if fr.Status != FeatureMismatch {
		t.Errorf("got %s, want mismatch", fr.Status)
	}
	if fr.Declared != "false" || fr.Live != "true" {
		t.Errorf("declared/live not reported: %+v", fr)
	}
	if !fr.HasDrift() {
		t.Error("expected drift")
	}
}

func TestReconcileSubnetFeaturesAbsentAndDisabledIsInSync(t *testing.T) {
	sn := Subnet{Name: "swiss", ID: testSubnetID}
	fr := ReconcileSubnetFeatures(sn, SubnetFeatures{SevEnabled: nil})
	if fr.Status != FeatureInSync {
		t.Errorf("got %s, want in-sync", fr.Status)
	}
	if fr.HasDrift() {
		t.Error("undeclared and off must not be drift")
	}
}

func TestReconcileSubnetFeaturesMatch(t *testing.T) {
	sn := Subnet{Name: "swiss", ID: testSubnetID, SevEnabled: true}
	fr := ReconcileSubnetFeatures(sn, SubnetFeatures{SevEnabled: new(true)})
	if fr.Status != FeatureInSync {
		t.Errorf("got %s, want in-sync", fr.Status)
	}
	if fr.HasDrift() {
		t.Error("matching sev_enabled must not be drift")
	}
}

func TestReconcileSubnetFeaturesMismatch(t *testing.T) {
	sn := Subnet{Name: "swiss", ID: testSubnetID, SevEnabled: true}
	fr := ReconcileSubnetFeatures(sn, SubnetFeatures{SevEnabled: new(false)})
	if fr.Status != FeatureMismatch {
		t.Errorf("got %s, want mismatch", fr.Status)
	}
	if !fr.HasDrift() {
		t.Error("expected drift")
	}
	if fr.Declared != "true" || fr.Live != "false" {
		t.Errorf("declared/live not reported: %+v", fr)
	}
}

// An absent sev_enabled on the live record is false, not unknown: the registry
// omits the field when the feature is off.
func TestReconcileSubnetFeaturesLiveAbsentIsFalse(t *testing.T) {
	sn := Subnet{Name: "swiss", ID: testSubnetID, SevEnabled: true}
	fr := ReconcileSubnetFeatures(sn, SubnetFeatures{SevEnabled: nil})
	if fr.Status != FeatureMismatch {
		t.Errorf("got %s, want mismatch", fr.Status)
	}
	if fr.Live != "false" {
		t.Errorf("live absent must read as false, got %q", fr.Live)
	}
}
