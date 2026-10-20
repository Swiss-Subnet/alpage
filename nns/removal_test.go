package nns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/registry"
	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
)

const (
	testNodeA = "dchi6-uidam-bqgay-dambq-gayda-mbqga-ydamb-qgayd-ambqg-aydam-bqg"
	testNodeB = "5ffj3-jarcq-lruhj-aemtc-sla"
	testOpA   = "hsi7b-rl4wt-lum3m-ophfi-oxgx5-u2q7r-ak7ag-nyiik-c4gam-epo2r-3qe"
	testNPA   = "ucjqj-jmbj3-rs4aq-ekzpw-ltjs3-zrcma-t6r3t-m5wxc-j5yrj-unwoj-mae"
)

// specFromBody loads a one-proposal config through the real loader, so these
// tests exercise the same decode path production does.
func specFromBody(t *testing.T, kind, body string) (Action, error) {
	t.Helper()
	dir := t.TempDir()
	cfg := fmt.Sprintf(`
proposal "fixture" {
  kind    = %q
  title   = "Fixture proposal"
  summary = "s"
  url     = "https://forum.dfinity.org/t/fixture"
%s
}
`, kind, body)
	path := filepath.Join(dir, "proposals.hcl")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "resources.hcl"), []byte(""), 0o644); err != nil {
		t.Fatalf("write resources: %v", err)
	}
	spec, err := LoadSpec(path, "fixture")
	if err != nil {
		return nil, err
	}
	return spec.Action()
}

func mustPrincipal(t *testing.T, s string) principal.Principal {
	t.Helper()
	p, err := principal.Decode(s)
	if err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return p
}

// Mainnet 142599 removes nodes via ExecuteNnsFunction 18.
func TestRemoveNodesPayload(t *testing.T) {
	a := RemoveNodesAction{
		Meta:    Meta{Title: "t", Summary: "s", URL: "u"},
		NodeIDs: []principal.Principal{mustPrincipal(t, testNodeA), mustPrincipal(t, testNodeB)},
	}
	if a.Kind() != "remove_nodes" {
		t.Errorf("kind = %q", a.Kind())
	}
	if a.NnsFunction() != governancepb.NnsFunction_NNS_FUNCTION_REMOVE_NODES {
		t.Errorf("nns function = %v, want REMOVE_NODES", a.NnsFunction())
	}
	blob, err := a.PayloadBlob()
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	var p registry.RemoveNodesPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if len(p.NodeIds) != 2 || p.NodeIds[0].Encode() != testNodeA {
		t.Errorf("node ids did not round-trip: %+v", p.NodeIds)
	}
}

// Mainnet 142603 removes operators by principal via ExecuteNnsFunction 23. The
// payload carries them under node_operator_principals_to_remove, not the
// deprecated raw-bytes field.
func TestRemoveNodeOperatorsPayload(t *testing.T) {
	a := RemoveNodeOperatorsAction{
		Meta:        Meta{Title: "t", Summary: "s", URL: "u"},
		OperatorIDs: []principal.Principal{mustPrincipal(t, testOpA)},
	}
	if a.NnsFunction() != governancepb.NnsFunction_NNS_FUNCTION_REMOVE_NODE_OPERATORS {
		t.Errorf("nns function = %v, want REMOVE_NODE_OPERATORS", a.NnsFunction())
	}
	blob, err := a.PayloadBlob()
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	var p registry.RemoveNodeOperatorsPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if p.NodeOperatorPrincipalsToRemove == nil {
		t.Fatal("operators must go in node_operator_principals_to_remove")
	}
	got := p.NodeOperatorPrincipalsToRemove.Principals
	if len(got) != 1 || got[0].Encode() != testOpA {
		t.Errorf("operator ids did not round-trip: %+v", got)
	}
}

// Mainnet 142930 removes a provider via the native AddOrRemoveNodeProvider
// action, so it carries no NNS function and no ExecuteNnsFunction wrapper.
func TestRemoveNodeProviderIsNativeAction(t *testing.T) {
	a := RemoveNodeProviderAction{
		Meta:       Meta{Title: "t", Summary: "s", URL: "u"},
		ProviderID: mustPrincipal(t, testNPA),
	}
	act, err := a.ProposalAction()
	if err != nil {
		t.Fatalf("proposal action: %v", err)
	}
	if act.ExecuteNnsFunction != nil {
		t.Error("a native action must not be wrapped in ExecuteNnsFunction")
	}
	if act.AddOrRemoveNodeProvider == nil {
		t.Fatal("expected an AddOrRemoveNodeProvider action")
	}
	change := act.AddOrRemoveNodeProvider.Change
	if change == nil || change.ToRemove == nil {
		t.Fatalf("expected a ToRemove change, got %+v", change)
	}
	if change.ToAdd != nil {
		t.Error("ToRemove and ToAdd are exclusive")
	}
	if id := change.ToRemove.Id; id == nil || id.Encode() != testNPA {
		t.Errorf("provider id did not survive: %+v", id)
	}
}

// The ExecuteNnsFunction kinds still build the wrapper, so adding a native kind
// did not change how the existing shape is encoded.
func TestExecuteNnsFunctionKindsStillWrap(t *testing.T) {
	a := RemoveNodesAction{
		Meta:    Meta{Title: "t"},
		NodeIDs: []principal.Principal{mustPrincipal(t, testNodeA)},
	}
	act, err := a.ProposalAction()
	if err != nil {
		t.Fatalf("proposal action: %v", err)
	}
	if act.ExecuteNnsFunction == nil {
		t.Fatal("expected an ExecuteNnsFunction wrapper")
	}
	if got := act.ExecuteNnsFunction.NnsFunction; got != int32(governancepb.NnsFunction_NNS_FUNCTION_REMOVE_NODES) {
		t.Errorf("nns function = %d, want 18", got)
	}
}

// An empty removal list would submit a proposal that does nothing, so it is
// rejected at load rather than at the NNS.
func TestRemovalKindsRejectEmptyLists(t *testing.T) {
	for _, tc := range []struct {
		name string
		hcl  string
	}{
		{"remove_nodes", `remove_nodes {}`},
		{"remove_node_operators", `remove_node_operators { operators = [] }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := specFromBody(t, tc.name, tc.hcl)
			if err == nil {
				t.Fatal("expected an error for an empty removal list")
			}
			if !strings.Contains(err.Error(), "at least one") {
				t.Errorf("error should say what is missing, got: %v", err)
			}
		})
	}
}

func TestRemoveNodesDecodesFromHCL(t *testing.T) {
	a, err := specFromBody(t, "remove_nodes", `
remove_nodes {
  node { id = "`+testNodeA+`" }
  node { id = "`+testNodeB+`" }
}`)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := a.(RemoveNodesAction)
	if !ok {
		t.Fatalf("got %T, want RemoveNodesAction", a)
	}
	if len(got.NodeIDs) != 2 {
		t.Errorf("got %d nodes, want 2", len(got.NodeIDs))
	}
}

func TestRemoveNodeProviderDecodesFromHCL(t *testing.T) {
	a, err := specFromBody(t, "remove_node_provider", `
remove_node_provider {
  node_provider_id = "`+testNPA+`"
}`)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := a.(RemoveNodeProviderAction)
	if !ok {
		t.Fatalf("got %T, want RemoveNodeProviderAction", a)
	}
	if got.ProviderID.Encode() != testNPA {
		t.Errorf("provider = %s, want %s", got.ProviderID.Encode(), testNPA)
	}
}
