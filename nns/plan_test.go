package nns

import (
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
)

var (
	nodeA   = principal.MustDecode("3wbrf-zokqb-6euxi-6lxxo-i5tia-4742s-7jfsj-touui-qwzbm-7rmdw-nae")
	nodeB   = principal.MustDecode("au6oc-imc3w-ssdnk-lzy6e-6fgeh-ejwch-bqohf-vj624-k5xfl-77rpz-xqe")
	nodeC   = principal.MustDecode("eaaef-crr36-d3ou2-fuyyz-mluny-cfm3r-zu4rf-22dhz-njgck-jres2-xqe")
	subnetX = principal.MustDecode("3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe")
)

func TestPlanMembershipClean(t *testing.T) {
	// current = {A}. Add C (new), remove A (present). No warnings.
	r := MembershipProposal{
		SubnetID:      subnetX,
		NodeIDsAdd:    []principal.Principal{nodeC},
		NodeIDsRemove: []principal.Principal{nodeA},
	}
	plan := PlanMembership(r, []string{nodeA.Encode()})
	if plan.HasWarnings() {
		t.Errorf("clean plan should have no warnings: %+v", plan.Ops)
	}
	if len(plan.Ops) != 2 {
		t.Fatalf("want 2 ops, got %d", len(plan.Ops))
	}
}

func TestPlanMembershipNoopAdd(t *testing.T) {
	// Add B when B is already a member -> warning.
	r := MembershipProposal{NodeIDsAdd: []principal.Principal{nodeB}}
	plan := PlanMembership(r, []string{nodeB.Encode()})
	if !plan.HasWarnings() {
		t.Fatal("adding an existing member should warn")
	}
	if plan.Ops[0].Kind != OpAdd || !plan.Ops[0].Warn {
		t.Errorf("op = %+v, want add with Warn", plan.Ops[0])
	}
}

func TestPlanMembershipPhantomRemove(t *testing.T) {
	// Remove C when C is not a member -> warning.
	r := MembershipProposal{NodeIDsRemove: []principal.Principal{nodeC}}
	plan := PlanMembership(r, []string{nodeA.Encode(), nodeB.Encode()})
	if !plan.HasWarnings() {
		t.Fatal("removing a non-member should warn")
	}
	if plan.Ops[0].Kind != OpRemove || !plan.Ops[0].Warn {
		t.Errorf("op = %+v, want remove with Warn", plan.Ops[0])
	}
}

func TestPlanAllNoOp(t *testing.T) {
	// Remove A and B when neither is a member -> every op is a no-op.
	r := MembershipProposal{NodeIDsRemove: []principal.Principal{nodeA, nodeB}}
	plan := PlanMembership(r, []string{nodeC.Encode()})
	if !plan.AllNoOp() {
		t.Errorf("all-phantom plan should be AllNoOp: %+v", plan.Ops)
	}
	if !plan.HasWarnings() {
		t.Error("all-no-op plan must also report warnings")
	}
}

func TestPlanNotAllNoOp(t *testing.T) {
	// Remove A (member, real) and B (not a member, no-op): mixed, not all-no-op.
	r := MembershipProposal{NodeIDsRemove: []principal.Principal{nodeA, nodeB}}
	plan := PlanMembership(r, []string{nodeA.Encode()})
	if plan.AllNoOp() {
		t.Error("a plan with one real op is not AllNoOp")
	}
}

func TestPlanEmptyNotAllNoOp(t *testing.T) {
	if (MembershipPlan{}).AllNoOp() {
		t.Error("an empty plan should not be AllNoOp")
	}
}

func TestMembershipPreflightClean(t *testing.T) {
	// Add C (new), remove A (present): no warnings -> empty report, Clean.
	plan := PlanMembership(MembershipProposal{
		NodeIDsAdd:    []principal.Principal{nodeC},
		NodeIDsRemove: []principal.Principal{nodeA},
	}, []string{nodeA.Encode()})
	pf := membershipPreflight(plan)
	if pf.Level != PreflightClean {
		t.Errorf("clean plan should be Clean, got %v", pf.Level)
	}
	if pf.Report != "" {
		t.Errorf("clean plan should have an empty report, got %q", pf.Report)
	}
}

func TestMembershipPreflightWarn(t *testing.T) {
	// Remove A (member, real) and B (phantom): mixed -> Warn, not NoOp.
	plan := PlanMembership(MembershipProposal{
		NodeIDsRemove: []principal.Principal{nodeA, nodeB},
	}, []string{nodeA.Encode()})
	pf := membershipPreflight(plan)
	if pf.Level != PreflightWarn {
		t.Errorf("a mixed plan with one real op must be Warn, got %v", pf.Level)
	}
	if pf.Report == "" {
		t.Error("a warning plan must render a report")
	}
}

func TestMembershipPreflightNoOp(t *testing.T) {
	// Remove A and B when neither is a member: every op is a no-op -> NoOp.
	plan := PlanMembership(MembershipProposal{
		NodeIDsRemove: []principal.Principal{nodeA, nodeB},
	}, []string{nodeC.Encode()})
	pf := membershipPreflight(plan)
	if pf.Level != PreflightNoOp {
		t.Errorf("an all-no-op plan must be NoOp, got %v", pf.Level)
	}
}

// elected is the elected-version set as the explorer reports it; "newver" and
// "abc123" are elected unless a test says otherwise.
var elected = ElectedVersions{
	IDs:    map[string]bool{"newver": true, "abc123": true},
	Source: SourceExplorer,
}

func TestDeployGuestosPlanSameVersionIsNoOp(t *testing.T) {
	pf := planDeployGuestos("abc123", "abc123", elected, nil)
	if pf.Level != PreflightNoOp {
		t.Errorf("deploying the version the subnet already runs is a no-op, got %v", pf.Level)
	}
	if pf.Report == "" {
		t.Error("expected a report explaining the no-op")
	}
}

func TestDeployGuestosPlanNewVersionProceeds(t *testing.T) {
	pf := planDeployGuestos("newver", "abc123", elected, nil)
	if pf.Level != PreflightClean {
		t.Errorf("a different target version is a real upgrade, got %v", pf.Level)
	}
	if pf.Report == "" {
		t.Error("expected a report showing current -> target")
	}
}

func TestDeployGuestosPlanUnelectedBlocks(t *testing.T) {
	// A target the NNS has not elected can never execute: refuse it.
	pf := planDeployGuestos("typo0", "abc123", elected, nil)
	if pf.Level != PreflightNoOp {
		t.Errorf("an unelected target must be refused, got %v", pf.Level)
	}
	if !strings.Contains(pf.Report, "not elected") {
		t.Errorf("report should say the version is not elected: %q", pf.Report)
	}
}

func TestDeployGuestosPlanUnelectedNamesSource(t *testing.T) {
	// The elected set is not a trustless read; the report must say where it came from.
	pf := planDeployGuestos("typo0", "abc123", elected, nil)
	if !strings.Contains(pf.Report, string(SourceExplorer)) {
		t.Errorf("report must attribute the elected set to the explorer: %q", pf.Report)
	}
}

func TestDeployGuestosPlanReportsRelease(t *testing.T) {
	// When the dashboard resolves the target, plan names the release and the
	// election proposal, attributed to the dashboard.
	rel := &Release{
		VersionID:  "newver",
		Name:       "release-2026-07-23_04-21-base",
		ProposalID: 143165,
		Source:     SourceDashboard,
	}
	pf := planDeployGuestos("newver", "abc123", elected, rel)
	for _, want := range []string{"release-2026-07-23_04-21-base", "143165", string(SourceDashboard)} {
		if !strings.Contains(pf.Report, want) {
			t.Errorf("report %q missing %q", pf.Report, want)
		}
	}
}

func TestDeployGuestosPlanConfirmsElected(t *testing.T) {
	// The report must say the election was verified, not just stay silent.
	pf := planDeployGuestos("newver", "abc123", elected, nil)
	if !strings.Contains(pf.Report, "elected") {
		t.Errorf("report should confirm the target is elected: %q", pf.Report)
	}
	if !strings.Contains(pf.Report, string(SourceExplorer)) {
		t.Errorf("the confirmation must name its source: %q", pf.Report)
	}
}

func TestDeployGuestosPlanUncheckedSaysNothing(t *testing.T) {
	// No elected set and no override: nothing was checked, so claim nothing.
	pf := planDeployGuestos("newver", "abc123", ElectedVersions{}, nil)
	if strings.Contains(pf.Report, "elected") {
		t.Errorf("an unchecked plan must not mention election: %q", pf.Report)
	}
}

func TestDeployGuestosPlanWithoutReleaseStillReports(t *testing.T) {
	// No release resolved: the version -> version line still renders.
	pf := planDeployGuestos("newver", "abc123", elected, nil)
	if !strings.Contains(pf.Report, "newver") {
		t.Errorf("report should still name the target version: %q", pf.Report)
	}
}

func TestPlanRenderShowsSignsAndWarnings(t *testing.T) {
	r := MembershipProposal{
		NodeIDsAdd:    []principal.Principal{nodeB}, // already member -> warn
		NodeIDsRemove: []principal.Principal{nodeA}, // member -> ok
	}
	plan := PlanMembership(r, []string{nodeA.Encode(), nodeB.Encode()})
	out := plan.Render()
	if !strings.Contains(out, "+ "+nodeB.Encode()) {
		t.Errorf("add line missing + sign: %q", out)
	}
	if !strings.Contains(out, "- "+nodeA.Encode()) {
		t.Errorf("remove line missing - sign: %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "warning") {
		t.Errorf("expected a warning marker in %q", out)
	}
}
