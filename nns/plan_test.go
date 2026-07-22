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

func TestPlanResizeClean(t *testing.T) {
	// current = {A}. Add C (new), remove A (present). No warnings.
	r := ResizeProposal{
		SubnetID:      subnetX,
		NodeIDsAdd:    []principal.Principal{nodeC},
		NodeIDsRemove: []principal.Principal{nodeA},
	}
	plan := PlanResize(r, []string{nodeA.Encode()})
	if plan.HasWarnings() {
		t.Errorf("clean plan should have no warnings: %+v", plan.Ops)
	}
	if len(plan.Ops) != 2 {
		t.Fatalf("want 2 ops, got %d", len(plan.Ops))
	}
}

func TestPlanResizeNoopAdd(t *testing.T) {
	// Add B when B is already a member -> warning.
	r := ResizeProposal{NodeIDsAdd: []principal.Principal{nodeB}}
	plan := PlanResize(r, []string{nodeB.Encode()})
	if !plan.HasWarnings() {
		t.Fatal("adding an existing member should warn")
	}
	if plan.Ops[0].Kind != OpAdd || !plan.Ops[0].Warn {
		t.Errorf("op = %+v, want add with Warn", plan.Ops[0])
	}
}

func TestPlanResizePhantomRemove(t *testing.T) {
	// Remove C when C is not a member -> warning.
	r := ResizeProposal{NodeIDsRemove: []principal.Principal{nodeC}}
	plan := PlanResize(r, []string{nodeA.Encode(), nodeB.Encode()})
	if !plan.HasWarnings() {
		t.Fatal("removing a non-member should warn")
	}
	if plan.Ops[0].Kind != OpRemove || !plan.Ops[0].Warn {
		t.Errorf("op = %+v, want remove with Warn", plan.Ops[0])
	}
}

func TestPlanRenderShowsSignsAndWarnings(t *testing.T) {
	r := ResizeProposal{
		NodeIDsAdd:    []principal.Principal{nodeB}, // already member -> warn
		NodeIDsRemove: []principal.Principal{nodeA}, // member -> ok
	}
	plan := PlanResize(r, []string{nodeA.Encode(), nodeB.Encode()})
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
