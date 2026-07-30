package nns

import (
	"os"
	"path/filepath"
	"testing"
)

// The test cloud engine on mainnet, and its live admin set.
const testEngineID = "nct3m-umxsm-gjtq2-vvk4r-zpmh7-ykldb-wb3kn-zfkr4-6mdvr-6y4mi-6qe"

var testEngineAdmins = []string{
	"fwida-eqaaa-aaabc-qaaba-cai",
	"bct5z-vccu4-6q4t2-3lb6l-wm43p-ulppt-o5sqq-w6het-rthdz-qp4yn-fqe",
	"3qi7c-7vlyz-6gxgq-yqavi-laryu-hj622-vwx2h-jmngg-psayx-kquuq-qae",
}

func liveEngine() SubnetRecordFacts {
	return SubnetRecordFacts{
		Type:         SubnetTypeCloudEngine,
		CostSchedule: CostScheduleFree,
		Admins:       testEngineAdmins,
	}
}

// Omitting type means application, so a cloud engine that is not declared as
// one is drift rather than silently accepted.
func TestReconcileSubnetTypeAbsentAssertsApplication(t *testing.T) {
	sn := Subnet{Name: "engine", ID: testEngineID}
	rs := ReconcileSubnetRecord(sn, liveEngine())
	tr := rs.Type
	if tr.Status != FeatureMismatch {
		t.Errorf("got %s, want mismatch", tr.Status)
	}
	if tr.Declared != "application" || tr.Live != "cloud_engine" {
		t.Errorf("declared/live not reported: %+v", tr)
	}
}

// A fully declared engine matching the live record must not drift. Admins are
// declared here too: omitting them asserts none, which is its own drift (see
// TestReconcileSubnetAdminsAbsentAssertsNone).
func TestReconcileSubnetTypeCloudEngineMatches(t *testing.T) {
	sn := Subnet{Name: "engine", ID: testEngineID, Type: "cloud_engine", Admins: testEngineAdmins}
	rs := ReconcileSubnetRecord(sn, liveEngine())
	if rs.Type.Status != FeatureInSync {
		t.Errorf("got %s, want in-sync", rs.Type.Status)
	}
	if rs.HasDrift() {
		t.Errorf("a matching engine must not drift: %+v", rs)
	}
}

// Admin order is not meaningful on-chain, so it must not read as drift.
func TestReconcileSubnetAdminsIgnoresOrder(t *testing.T) {
	sn := Subnet{
		Name: "engine", ID: testEngineID, Type: "cloud_engine",
		Admins: []string{testEngineAdmins[2], testEngineAdmins[0], testEngineAdmins[1]},
	}
	rs := ReconcileSubnetRecord(sn, liveEngine())
	if rs.Admins.Status != FeatureInSync {
		t.Errorf("got %s, want in-sync: %+v", rs.Admins.Status, rs.Admins)
	}
}

func TestReconcileSubnetAdminsReportsAddedAndRemoved(t *testing.T) {
	const extra = "2vxsx-fae"
	sn := Subnet{
		Name: "engine", ID: testEngineID, Type: "cloud_engine",
		// Drops the third live admin, declares one the registry does not have.
		Admins: []string{testEngineAdmins[0], testEngineAdmins[1], extra},
	}
	rs := ReconcileSubnetRecord(sn, liveEngine())
	if rs.Admins.Status != FeatureMismatch {
		t.Fatalf("got %s, want mismatch", rs.Admins.Status)
	}
	if len(rs.Admins.Missing) != 1 || rs.Admins.Missing[0] != extra {
		t.Errorf("declared-but-absent not reported: %+v", rs.Admins.Missing)
	}
	if len(rs.Admins.Unexpected) != 1 || rs.Admins.Unexpected[0] != testEngineAdmins[2] {
		t.Errorf("live-but-undeclared not reported: %+v", rs.Admins.Unexpected)
	}
	if !rs.HasDrift() {
		t.Error("expected drift")
	}
}

// Declaring no admins asserts none, so an engine with admins on-chain drifts.
// Otherwise dropping the block would silently stop reconciling them.
func TestReconcileSubnetAdminsAbsentAssertsNone(t *testing.T) {
	sn := Subnet{Name: "engine", ID: testEngineID, Type: "cloud_engine"}
	rs := ReconcileSubnetRecord(sn, liveEngine())
	if rs.Admins.Status != FeatureMismatch {
		t.Errorf("got %s, want mismatch", rs.Admins.Status)
	}
	if len(rs.Admins.Unexpected) != len(testEngineAdmins) {
		t.Errorf("want all %d live admins unexpected, got %+v", len(testEngineAdmins), rs.Admins.Unexpected)
	}
}

// The registry rejects a cloud engine that is not on the free cost schedule
// (check_subnet_cost_schedule_invariant), so catch it before a proposal is cut.
func TestValidateSubnetCloudEngineRequiresFreeSchedule(t *testing.T) {
	sn := Subnet{Name: "engine", ID: testEngineID, Type: "cloud_engine", CostSchedule: "normal"}
	if err := ValidateSubnet(sn); err == nil {
		t.Fatal("expected an error for a cloud engine on the normal schedule")
	}
}

func TestValidateSubnetCloudEngineFreeScheduleOK(t *testing.T) {
	sn := Subnet{Name: "engine", ID: testEngineID, Type: "cloud_engine"}
	if err := ValidateSubnet(sn); err != nil {
		t.Fatalf("cloud engine defaults must validate: %v", err)
	}
}

// check_subnet_admins_invariant allows admins only on a free-schedule
// application subnet (a rented one) or a cloud engine, so an ordinary
// application subnet on the normal schedule may not carry them.
func TestValidateSubnetAdminsRequireFreeSchedule(t *testing.T) {
	sn := Subnet{Name: "app", ID: testEngineID, Admins: testEngineAdmins}
	if err := ValidateSubnet(sn); err == nil {
		t.Fatal("expected an error for admins on a normal-schedule application subnet")
	}
}

// A rented subnet is application + free, and may carry admins.
func TestValidateSubnetRentedAdminsOK(t *testing.T) {
	sn := Subnet{
		Name: "rented", ID: testEngineID,
		Type: "application", CostSchedule: "free", Admins: testEngineAdmins,
	}
	if err := ValidateSubnet(sn); err != nil {
		t.Fatalf("a rented subnet may carry admins: %v", err)
	}
}

func TestValidateSubnetAdminsRejectedOnSystem(t *testing.T) {
	sn := Subnet{Name: "sys", ID: testEngineID, Type: "system", Admins: testEngineAdmins}
	if err := ValidateSubnet(sn); err == nil {
		t.Fatal("expected an error for subnet_admins on a system subnet")
	}
}

// MAX_SUBNET_ADMINS in the registry invariants.
func TestValidateSubnetAdminsMax(t *testing.T) {
	admins := make([]string, 11)
	for i := range admins {
		admins[i] = testEngineAdmins[0]
	}
	sn := Subnet{Name: "engine", ID: testEngineID, Type: "cloud_engine", Admins: admins}
	if err := ValidateSubnet(sn); err == nil {
		t.Fatal("expected an error for more than 10 subnet admins")
	}
}

// loadResources must reject an invalid subnet, so a bad declaration fails at
// load rather than surviving to reconcile.
func TestLoadResourcesValidatesSubnets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resources.hcl")
	// A cloud engine on the normal schedule: rejected by the registry.
	hcl := `subnet "engine" {
  id            = "` + testEngineID + `"
  type          = "cloud_engine"
  cost_schedule = "normal"
}
`
	if err := os.WriteFile(path, []byte(hcl), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadResources(path); err == nil {
		t.Fatal("expected loadResources to reject a cloud_engine on the normal schedule")
	}
}

func TestValidateSubnetRejectsUnknownType(t *testing.T) {
	sn := Subnet{Name: "engine", ID: testEngineID, Type: "cloud-engine"}
	if err := ValidateSubnet(sn); err == nil {
		t.Fatal("expected an error for an unknown subnet type")
	}
}

// A cost schedule mismatch against the live record is drift, not just a config
// error: an engine moved off Free on-chain is a fact worth surfacing.
func TestReconcileSubnetCostScheduleMismatch(t *testing.T) {
	sn := Subnet{Name: "engine", ID: testEngineID, Type: "cloud_engine"}
	live := liveEngine()
	live.CostSchedule = CostScheduleNormal
	rs := ReconcileSubnetRecord(sn, live)
	if rs.CostSchedule.Status != FeatureMismatch {
		t.Errorf("got %s, want mismatch", rs.CostSchedule.Status)
	}
	if !rs.HasDrift() {
		t.Error("expected drift")
	}
}
