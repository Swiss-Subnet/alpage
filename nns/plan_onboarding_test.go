package nns

import (
	"strings"
	"testing"
)

// do_add_or_remove_data_centers panics on an id the registry already holds,
// which happens after the NNS has adopted the proposal.
func TestPlanAddDataCentersBlocksExistingID(t *testing.T) {
	p := planAddDataCenters(
		[]DataCenterSpec{
			{ID: "zh3", Region: "Europe,CH,Zurich", Owner: "Cloud Engines AG"},
			{ID: "zh4", Region: "Europe,CH,Zurich", Owner: "Cloud Engines AG"},
		},
		map[string]DataCenterSpec{
			"zh3": {ID: "zh3", Region: "Europe,CH,Geneva", Owner: "Someone Else AG"},
		},
	)
	if !p.HasBlocking() {
		t.Fatal("an already-registered data center must block")
	}
	if len(p.Issues) != 1 || p.Issues[0].Target != "zh3" {
		t.Fatalf("expected one issue for zh3, got %+v", p.Issues)
	}
	// The report has to say what differs, since that is how an operator tells a
	// resubmission apart from a collision with someone else's data center.
	if d := p.Issues[0].Detail; !strings.Contains(d, "Geneva") || !strings.Contains(d, "Someone Else AG") {
		t.Errorf("detail should name the differing fields, got %q", d)
	}
}

// A record over the canister's length limit is dropped from the batch while the
// proposal still reports success, so preflight has to catch it.
func TestPlanAddDataCentersBlocksOverlongField(t *testing.T) {
	p := planAddDataCenters(
		[]DataCenterSpec{{ID: "zh3", Region: strings.Repeat("x", 256), Owner: "Cloud Engines AG"}},
		nil,
	)
	if !p.HasBlocking() {
		t.Fatal("an overlong region must block")
	}
	if d := p.Issues[0].Detail; !strings.Contains(d, "region") {
		t.Errorf("detail should name the offending field, got %q", d)
	}
}

func TestPlanAddDataCentersCleanWhenNew(t *testing.T) {
	p := planAddDataCenters(
		[]DataCenterSpec{{ID: "zh3", Region: "Europe,CH,Zurich", Owner: "Cloud Engines AG"}},
		nil,
	)
	if len(p.Issues) != 0 {
		t.Fatalf("registering a new data center has no issues, got %+v", p.Issues)
	}
	if preconditionPreflight(p).Level != PreflightClean {
		t.Error("a new data center must preflight clean")
	}
}

// Governance rejects ToAdd for a principal already on the provider list.
func TestPlanAddNodeProviderBlocksDuplicate(t *testing.T) {
	p := planAddNodeProvider("np-1", true)
	if !p.HasBlocking() {
		t.Error("an already-registered provider must block")
	}
}

// Governance pays a provider with no reward_account into the default account of
// its principal, so the omission is not worth reporting.
func TestPlanAddNodeProviderAcceptsMissingRewardAccount(t *testing.T) {
	p := planAddNodeProvider("np-1", false)
	if preconditionPreflight(p).Level != PreflightClean {
		t.Errorf("a provider with no reward account must preflight clean, got %+v", p.Issues)
	}
}

// The data center and the provider are the operator's two preconditions, and
// they are what forces this proposal to go last.
func TestPlanAddNodeOperatorBlocksMissingReferents(t *testing.T) {
	p := planAddNodeOperator("op-1", "np-1", "zh3", false, false, false, 10)
	if len(p.Issues) != 2 {
		t.Fatalf("a missing data center and provider are two issues, got %+v", p.Issues)
	}
	joined := p.Issues[0].Detail + " " + p.Issues[1].Detail
	if !strings.Contains(joined, "add_data_centers") || !strings.Contains(joined, "add_node_provider") {
		t.Errorf("each issue should name the proposal that fixes it, got %q", joined)
	}
}

// Zero allowance is the Cloud Engine case, so it is a note rather than a block.
func TestPlanAddNodeOperatorZeroAllowanceIsNotBlocking(t *testing.T) {
	p := planAddNodeOperator("op-1", "np-1", "zh3", false, true, true, 0)
	if p.HasBlocking() {
		t.Error("a zero allowance is a legitimate configuration")
	}
	if len(p.Issues) != 1 {
		t.Fatalf("expected the zero allowance to be reported, got %+v", p.Issues)
	}
}

func TestPlanAddNodeOperatorCleanWhenReady(t *testing.T) {
	p := planAddNodeOperator("op-1", "np-1", "zh3", false, true, true, 28)
	if len(p.Issues) != 0 {
		t.Fatalf("a fully satisfied operator add has no issues, got %+v", p.Issues)
	}
}
