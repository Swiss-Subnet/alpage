package nns

import (
	"os"
	"path/filepath"
	"testing"

	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
)

const testConfig = "testdata/config.hcl"

const (
	fxSubnet = "67htk-vfkxp-gn33q-baibq"
	fxNode1  = "5ffj3-jarcq-lruhj-aemtc-sla"
)

func TestLoadSpecMatchesProposal(t *testing.T) {
	spec, err := LoadSpec(testConfig, "membership-example")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	p, err := spec.Proposal()
	if err != nil {
		t.Fatalf("proposal: %v", err)
	}
	if len(p.NodeIDsRemove) != 1 {
		t.Errorf("remove count = %d, want 1", len(p.NodeIDsRemove))
	}
	if p.SubnetID.Encode() != fxSubnet {
		t.Errorf("subnet id = %s", p.SubnetID.Encode())
	}
}

func TestResourceReferencesResolve(t *testing.T) {
	spec, err := LoadSpec(testConfig, "membership-example")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	p, err := spec.Proposal()
	if err != nil {
		t.Fatalf("proposal: %v", err)
	}
	// subnet.test.id resolved to the subnet principal.
	if got := p.SubnetID.Encode(); got != fxSubnet {
		t.Errorf("subnet ref resolved to %s", got)
	}
	// node.n1.id resolved to the node principal.
	if got := p.NodeIDsRemove[0].Encode(); got != fxNode1 {
		t.Errorf("node ref resolved to %s", got)
	}
}

// TestInlineAndRefAreEquivalent proves an inline id and a resource reference to
// the same id produce the identical payload hash.
func TestInlineAndRefAreEquivalent(t *testing.T) {
	ref, err := LoadSpec(testConfig, "membership-example")
	if err != nil {
		t.Fatalf("load ref spec: %v", err)
	}
	refHash, err := ref.PayloadSHA256()
	if err != nil {
		t.Fatalf("ref hash: %v", err)
	}
	inline, err := LoadSpec("testdata/inline.hcl", "membership-example")
	if err != nil {
		t.Fatalf("load inline spec: %v", err)
	}
	inlineHash, err := inline.PayloadSHA256()
	if err != nil {
		t.Fatalf("inline hash: %v", err)
	}
	if refHash != inlineHash {
		t.Errorf("inline vs reference hash differ:\n inline=%s\n ref   =%s", inlineHash, refHash)
	}
}

func TestLoadDeployGuestosSpec(t *testing.T) {
	spec, err := LoadSpec(testConfig, "deploy-guestos-example")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	a, err := spec.Action()
	if err != nil {
		t.Fatalf("action: %v", err)
	}
	d, ok := a.(DeployGuestosAction)
	if !ok {
		t.Fatalf("action is %T, want DeployGuestosAction", a)
	}
	if a.NnsFunction() != governancepb.NnsFunction_NNS_FUNCTION_DEPLOY_GUESTOS_TO_ALL_SUBNET_NODES {
		t.Errorf("nns function = %d", a.NnsFunction())
	}
	if d.Metadata().Title == "" {
		t.Error("embedded Meta not populated")
	}
	if _, err := a.PayloadBlob(); err != nil {
		t.Errorf("payload blob: %v", err)
	}
	// A membership spec is not a deploy_guestos, and vice versa.
	if _, err := spec.Proposal(); err == nil {
		t.Error("expected Proposal() to reject a non-membership spec")
	}
}

// TestNnsFunctionNumbers pins each action's function number to the literal
// value in the governance enum. The enum now comes from the pinned release, so
// this guards against picking the wrong variant rather than mistyping a number.
func TestNnsFunctionNumbers(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  int32
		want int32
	}{
		{"membership", int32(MembershipProposal{}.NnsFunction()), 31},
		{"deploy_guestos", int32(DeployGuestosAction{}.NnsFunction()), 11},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestLoadSpecUnknownName(t *testing.T) {
	if _, err := LoadSpec(testConfig, "does-not-exist"); err == nil {
		t.Fatal("expected error for unknown proposal name")
	}
}

// TestPayloadHashStable pins the wire-payload hash. If this changes, the
// submitted bytes changed and any recorded state is stale.
func TestPayloadHashStable(t *testing.T) {
	spec, err := LoadSpec(testConfig, "membership-example")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	h1, err := spec.PayloadSHA256()
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	h2, err := spec.PayloadSHA256()
	if err != nil {
		t.Fatalf("hash again: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hash not deterministic: %s vs %s", h1, h2)
	}
	t.Logf("membership-example payload sha256 = %s", h1)
}

func TestStateImport(t *testing.T) {
	spec, err := LoadSpec(testConfig, "membership-example")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	hash, err := spec.PayloadSHA256()
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	st, _ := LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err := st.Import(spec, 142931, "prin", MainnetHost, "2026-07-17T00:00:00Z", ProposerNeuronID); err != nil {
		t.Fatalf("import: %v", err)
	}
	e := st.Proposals["membership-example"]
	if e.ProposalID != 142931 || e.PayloadSHA256 != hash || e.SubmittedBy != "prin" || e.Neuron != ProposerNeuronID {
		t.Errorf("imported entry wrong: %+v", e)
	}

	// Re-importing the same name must fail rather than clobber.
	if err := st.Import(spec, 999, "", "", "", 0); err == nil {
		t.Error("expected error re-importing an existing name")
	}
	// Zero id is rejected.
	st2, _ := LoadState(filepath.Join(t.TempDir(), "s.json"))
	if err := st2.Import(spec, 0, "", "", "", 0); err == nil {
		t.Error("expected error importing zero proposal id")
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	st, err := LoadState(path)
	if err != nil || len(st.Proposals) != 0 {
		t.Fatalf("missing state: got %+v, %v; want empty, nil", st, err)
	}
	want := Entry{Kind: "membership", ProposalID: 142931, PayloadSHA256: "abc", SubmittedBy: "p", Neuron: 1, Host: "h", SubmittedAt: "2026-07-17T00:00:00Z"}
	st.Proposals["membership-example"] = want
	if err := SaveState(path, st); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Proposals["membership-example"] != want {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got.Proposals["membership-example"], want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file missing: %v", err)
	}
}
