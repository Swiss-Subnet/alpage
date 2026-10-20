package nns

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/registry"
	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
)

// loadUpdateSubnet writes a config with one update_subnet proposal and returns
// the decoded action, so a test states only the block it cares about.
func loadUpdateSubnet(t *testing.T, block string) UpdateSubnetAction {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "proposals.hcl")
	src := `proposal "cfg" {
  kind  = "update_subnet"
  title = "t"
  update_subnet {
    subnet_id = "` + dryRunSeedSubnet + `"
` + block + `
  }
}
`
	if err := os.WriteFile(cfg, []byte(src), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	spec, err := LoadSpec(cfg, "cfg")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	a, err := spec.Action()
	if err != nil {
		t.Fatalf("action: %v", err)
	}
	u, ok := a.(UpdateSubnetAction)
	if !ok {
		t.Fatalf("action is %T, want UpdateSubnetAction", a)
	}
	return u
}

// The registry skips a field the payload leaves unset, so "declared empty" and
// "not declared" mean opposite things: one clears the subnet's list, the other
// leaves it alone. If HCL decoding collapsed them, a proposal that touches one
// setting would silently revoke every SSH key on the subnet.
func TestUpdateSubnetDistinguishesAbsentFromEmpty(t *testing.T) {
	absent := loadUpdateSubnet(t, `    is_halted = true`)
	if absent.Settings.SSHReadonlyAccess != nil {
		t.Errorf("undeclared ssh_readonly_access decoded as %v, want nil (untouched)", *absent.Settings.SSHReadonlyAccess)
	}

	empty := loadUpdateSubnet(t, `    ssh_readonly_access = []`)
	if empty.Settings.SSHReadonlyAccess == nil {
		t.Fatal("ssh_readonly_access = [] decoded as nil; clearing the list would be impossible")
	}
	if len(*empty.Settings.SSHReadonlyAccess) != 0 {
		t.Errorf("ssh_readonly_access = %v, want empty", *empty.Settings.SSHReadonlyAccess)
	}
}

// The payload must carry exactly the declared fields: anything else it sets
// would be written to the subnet record as a change nobody asked for.
func TestUpdateSubnetPayloadCarriesOnlyDeclaredFields(t *testing.T) {
	u := loadUpdateSubnet(t, `    is_halted               = true
    max_number_of_canisters = 42`)

	blob, err := u.PayloadBlob()
	if err != nil {
		t.Fatalf("PayloadBlob: %v", err)
	}
	var p registry.UpdateSubnetPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if p.IsHalted == nil || !*p.IsHalted {
		t.Error("is_halted not set in the payload")
	}
	if p.MaxNumberOfCanisters == nil || *p.MaxNumberOfCanisters != 42 {
		t.Errorf("max_number_of_canisters = %v, want 42", p.MaxNumberOfCanisters)
	}
	for name, set := range map[string]bool{
		"halt_at_cup_height":  p.HaltAtCupHeight != nil,
		"ssh_readonly_access": p.SshReadonlyAccess != nil,
		"ssh_backup_access":   p.SshBackupAccess != nil,
		"subnet_admins":       p.SubnetAdmins != nil,
		"features":            p.Features != nil,
		"resource_limits":     p.ResourceLimits != nil,
		"unit_delay_millis":   p.UnitDelayMillis != nil,
		"subnet_type":         p.SubnetType != nil,
	} {
		if set {
			t.Errorf("payload sets %s, which the proposal does not declare", name)
		}
	}
	if u.NnsFunction() != governancepb.NnsFunction_NNS_FUNCTION_UPDATE_CONFIG_OF_SUBNET {
		t.Errorf("NnsFunction = %v, want UPDATE_CONFIG_OF_SUBNET", u.NnsFunction())
	}
}

// A features block is all-or-nothing because the registry replaces the whole
// record: an omitted sub-field would be written as false. Requiring them makes
// that a load error instead of a silent feature flip.
func TestUpdateSubnetFeaturesRequiresEveryField(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "proposals.hcl")
	src := `proposal "cfg" {
  kind  = "update_subnet"
  title = "t"
  update_subnet {
    subnet_id = "` + dryRunSeedSubnet + `"
    features {
      canister_sandboxing = true
    }
  }
}
`
	if err := os.WriteFile(cfg, []byte(src), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadSpec(cfg, "cfg")
	if err == nil {
		t.Fatal("want an error for a features block missing http_requests and sev_enabled")
	}
	if !strings.Contains(err.Error(), "http_requests") {
		t.Errorf("error %q should name the missing field", err)
	}
}

// A proposal that declares no setting encodes and executes fine and changes
// nothing, so it is rejected where it is written rather than after it is voted.
func TestUpdateSubnetRejectsEmptySettings(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "proposals.hcl")
	src := `proposal "cfg" {
  kind  = "update_subnet"
  title = "t"
  update_subnet {
    subnet_id = "` + dryRunSeedSubnet + `"
  }
}
`
	if err := os.WriteFile(cfg, []byte(src), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadSpec(cfg, "cfg"); err == nil {
		t.Fatal("want an error for an update_subnet that declares no settings")
	}
}

// Empty decides whether a proposal is rejected for declaring nothing, and it
// lists its fields by hand: a setting added without extending it would make an
// update_subnet naming only that setting look empty and be refused at load.
func TestSubnetSettingsEmptyCoversEveryField(t *testing.T) {
	v := reflect.TypeOf(SubnetSettings{})
	for i := range v.NumField() {
		f := v.Field(i)
		if f.Type.Kind() != reflect.Ptr {
			t.Errorf("field %s is %s, not a pointer, so an unset value cannot mean untouched", f.Name, f.Type)
			continue
		}
		s := SubnetSettings{}
		reflect.ValueOf(&s).Elem().Field(i).Set(reflect.New(f.Type.Elem()))
		if s.Empty() {
			t.Errorf("Empty() is true with only %s set; add it to Empty()", f.Name)
		}
	}
}

// currentSubnet is a subnet record to diff against: halted false, one SSH key,
// and features with SEV on.
func currentSubnet() SubnetSettings {
	sev := true
	return currentSettings(&registry.SubnetRecord{
		IsHalted:             false,
		HaltAtCupHeight:      false,
		MaxNumberOfCanisters: 10,
		SshReadonlyAccess:    []string{"key-a"},
		UnitDelayMillis:      1000,
		Features: &registry.SubnetFeatures{
			CanisterSandboxing: true,
			HttpRequests:       true,
			SevEnabled:         &sev,
		},
	})
}

// Preflight's whole job here is telling a real change from a setting that
// already holds: a proposal whose every declared setting matches on-chain state
// changes nothing and apply refuses it.
func TestPlanUpdateSubnetGradesChangesAgainstCurrentState(t *testing.T) {
	cur := currentSubnet()
	halted, canisters := true, uint64(10)

	change := planUpdateSubnet("sn", SubnetSettings{IsHalted: &halted}, cur)
	if pf := updateSubnetPreflight(change); pf.Level != PreflightClean {
		t.Errorf("halting an unhalted subnet graded %v, want Clean:\n%s", pf.Level, pf.Report)
	}

	noop := planUpdateSubnet("sn", SubnetSettings{MaxNumberOfCanisters: &canisters}, cur)
	if !noop.AllNoOp() {
		t.Error("max_number_of_canisters already 10 should be a no-op")
	}
	if pf := updateSubnetPreflight(noop); pf.Level != PreflightNoOp {
		t.Errorf("all-no-op plan graded %v, want NoOp", pf.Level)
	}
}

// Turning sev_enabled off cannot execute: validate_sev_is_not_disabled panics
// after the proposal is adopted. Preflight has to name it before the vote. The
// trap is easy to walk into because features is replaced wholesale, so simply
// omitting sev_enabled reads as disabling it.
func TestPlanUpdateSubnetBlocksSevDisable(t *testing.T) {
	cur := currentSubnet()
	off := SubnetFeatureSet{CanisterSandboxing: true, HTTPRequests: true, SevEnabled: false}

	p := planUpdateSubnet("sn", SubnetSettings{Features: &off}, cur)
	if !p.HasBlocking() {
		t.Fatal("disabling sev_enabled should block")
	}
	report := p.Render()
	if !strings.Contains(report, "sev_enabled") || !strings.Contains(report, "BLOCKS") {
		t.Errorf("report should name the blocked field:\n%s", report)
	}

	on := SubnetFeatureSet{CanisterSandboxing: true, HTTPRequests: true, SevEnabled: true}
	if planUpdateSubnet("sn", SubnetSettings{Features: &on}, cur).HasBlocking() {
		t.Error("restating sev_enabled unchanged must be allowed; it is how the other features are updated")
	}
}

// Enabling SEV is permitted, so it must not block. It still traps at execution
// unless the nodes carry chip IDs, which the operator has to know before voting
// rather than after.
func TestPlanUpdateSubnetAllowsSevEnableWithWarning(t *testing.T) {
	cur := currentSubnet()
	cur.Features = &SubnetFeatureSet{CanisterSandboxing: true, HTTPRequests: true, SevEnabled: false}
	on := SubnetFeatureSet{CanisterSandboxing: true, HTTPRequests: true, SevEnabled: true}

	p := planUpdateSubnet("sn", SubnetSettings{Features: &on}, cur)
	if p.HasBlocking() {
		t.Error("enabling sev_enabled is allowed by the registry and must not block")
	}
	if report := p.Render(); !strings.Contains(report, "chip_id") {
		t.Errorf("report should warn about the chip_id requirement:\n%s", report)
	}
}

// Lists are replaced, not merged, so a declared list that drops existing entries
// is reported rather than applied quietly: revoking SSH access and forgetting to
// restate a key look identical in the payload.
func TestPlanUpdateSubnetFlagsClearedList(t *testing.T) {
	cur := currentSubnet()
	var none []string

	p := planUpdateSubnet("sn", SubnetSettings{SSHReadonlyAccess: &none}, cur)
	if pf := updateSubnetPreflight(p); pf.Level != PreflightWarn {
		t.Errorf("clearing a non-empty list graded %v, want Warn:\n%s", pf.Level, pf.Report)
	}
	if !strings.Contains(p.Render(), "replaced, not merged") {
		t.Errorf("report should explain the replacement:\n%s", p.Render())
	}
}

// The dry-run needs the real subnet record: update_subnet merges into it, so
// without the mirror it fails on a missing subnet instead of testing the payload.
func TestUpdateSubnetMirrorsItsSubnet(t *testing.T) {
	subnet := principal.MustDecode(dryRunSeedSubnet)
	u := UpdateSubnetAction{SubnetID: subnet}

	scope := u.MirrorScope()
	if len(scope.Subnets) != 1 || scope.Subnets[0].Encode() != subnet.Encode() {
		t.Errorf("scope = %+v, want just the target subnet", scope)
	}
}
