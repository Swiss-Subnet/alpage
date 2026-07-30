package nns

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/pocketic"
)

// startNNS boots a PocketIC instance, brings up the NNS with the given hotkey
// (empty for none), and advances the clock so governance seeds its RNG and the
// proposer neuron has voting power.
func startNNS(t *testing.T, hotkey principal.Principal) *NNS {
	t.Helper()
	n, _, _ := startNNSSeeded(t, hotkey, nil, false)
	return n
}

// startNNSSeeded is startNNS with optional subnet seeds and, when live, an HTTP
// gateway so the real HTTP-agent read paths (FetchSubnet*, Preflight) can reach
// the instance. Returns the NNS, its PocketIC client, and the gateway URL
// ("" when live is false).
func startNNSSeeded(t *testing.T, hotkey principal.Principal, seeds []SubnetSeed, live bool) (*NNS, *pocketic.Client, string) {
	t.Helper()
	return startNNSSeededWithProviders(t, hotkey, seeds, nil, live)
}

func startNNSSeededWithProviders(t *testing.T, hotkey principal.Principal, seeds []SubnetSeed, providers []ProviderSeed, live bool) (*NNS, *pocketic.Client, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping PocketIC-backed test in -short mode")
	}
	if os.Getenv("POCKET_IC_BIN") == "" {
		t.Skip("POCKET_IC_BIN not set; run inside nix develop")
	}
	c, err := pocketic.Start("")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	inst, err := c.NewInstance()
	if err != nil {
		t.Fatalf("instance: %v", err)
	}
	n, err := BringUpWithSubnetsAndProviders(c, inst, testController(), hotkey, seeds, providers)
	if err != nil {
		t.Fatalf("bring up: %v", err)
	}
	if err := c.SetTime(inst, 1_700_000_000_000_000_000); err != nil {
		t.Fatalf("set_time: %v", err)
	}
	if err := c.AutoProgress(inst); err != nil {
		t.Fatalf("auto_progress: %v", err)
	}
	for i := 0; i < 5; i++ {
		_ = c.Tick(inst)
	}
	var url string
	if live {
		url, err = c.MakeLive(inst)
		if err != nil {
			t.Fatalf("make live: %v", err)
		}
		t.Cleanup(func() { _ = c.StopGateway(inst) })
	}
	return n, c, url
}

// stubSources serves the two third-party sources deploy_guestos preflight
// consults: the registry explorer's replica_version_<id> records and the
// dashboard's election-proposal list. Only the listed versions get a record; any
// other lookup 404s, i.e. reads as unelected. Returns the options pointing
// preflight at the stub.
//
// Without this a preflight in tests would reach the public internet, which both
// fails without egress and answers about mainnet rather than the seeded state.
func stubSources(t *testing.T, elected ...string) []FetchOption {
	t.Helper()
	isElected := make(map[string]bool, len(elected))
	for _, v := range elected {
		isElected[v] = true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/records/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(path.Base(r.URL.Path), "replica_version_")
		if !isElected[id] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `[{"key":"replica_version_%s","version":1,"value":"CIA="}]`, id)
	})
	// No elections: releases stay unresolved, which preflight tolerates.
	mux.HandleFunc("/proposals", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return []FetchOption{WithExplorer(srv.URL), WithDashboard(srv.URL + "/proposals")}
}

// resizeFixture loads the self-contained resize fixture (the live config lives
// in a separate repo).
func resizeFixture(t *testing.T) ResizeProposal {
	t.Helper()
	spec, err := LoadSpec("testdata/golden_src/proposals.hcl", "resize-fixture")
	if err != nil {
		t.Fatalf("load resize-fixture spec: %v", err)
	}
	p, err := spec.Proposal()
	if err != nil {
		t.Fatalf("decode resize-fixture spec: %v", err)
	}
	return p
}

func assertResizeRendered(t *testing.T, rendered string) {
	t.Helper()
	for _, want := range []string{
		"change_subnet_membership",
		"wmzac-nabae", // fixture subnet.test
		"nodes removed (2)",
		"nodes added (0)",
		"uduew-qycai", // node.n1
		"dchi6-uidam", // inline removed id
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered proposal missing %q", want)
		}
	}
}

func TestSubmitResizeFixture(t *testing.T) {
	n := startNNS(t, principal.Principal{})
	neuron := n.ProposerNeuron()

	pid, err := n.SubmitResize(neuron, resizeFixture(t))
	if err != nil {
		t.Fatalf("submit resize: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	rendered := Render(pi)
	t.Logf("\n%s", rendered)

	assertResizeRendered(t, rendered)
	// The seeded neuron has voting power, so the proposal is adopted. This
	// harness deliberately brings up an empty registry (no seeds), so execution
	// then fails for lack of subnet records; that is fine here, as this test
	// only checks adoption + rendering. To exercise the read paths against a
	// populated registry, see BringUpWithSubnets / TestPreflightAgainstSeededSubnet.
	if pi.LatestTally == nil || pi.LatestTally.Yes == 0 {
		t.Errorf("expected a non-zero yes tally (adopted), got %+v", pi.LatestTally)
	}
}

// TestSubmitDeployGuestosFixture submits a deploy_guestos through real
// governance, which is the only check that alpage's NNS function number means
// what alpage thinks it means: governance stores the number we send, so reading
// the proposal back and finding it labelled deploy_guestos (rather than some
// other function) is ground truth no in-repo constant can fake.
func TestSubmitDeployGuestosFixture(t *testing.T) {
	n := startNNS(t, principal.Principal{})
	neuron := n.ProposerNeuron()

	d := DeployGuestosAction{
		Meta:             Meta{Title: "deploy guestos fixture", Summary: "e2e"},
		SubnetID:         subnetX,
		ReplicaVersionID: "0000000000000000000000000000000000000001",
	}
	pid, err := n.SubmitAs(n.Proposer, neuron, d)
	if err != nil {
		t.Fatalf("submit deploy_guestos: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	rendered := RenderVerbose(pi)
	t.Logf("\n%s", rendered)

	for _, want := range []string{
		"deploy_guestos_to_all_subnet_nodes",
		d.ReplicaVersionID,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered proposal missing %q", want)
		}
	}
	// The label comes from alpage's own funcName map, so it renders whatever we
	// named the constant; only the number is governance's record of what we sent.
	if !strings.Contains(rendered, "ExecuteNnsFunction #11") {
		t.Errorf("expected nns function 11 in rendered proposal, got:\n%s", rendered)
	}
}

func sortedEncode(ps []principal.Principal) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Encode()
	}
	return out
}

// TestPreflightAgainstSeededSubnet seeds the local registry with a subnet
// (membership + replica version), then exercises the production read paths over
// an HTTP gateway: FetchSubnetMembership/FetchSubnetReplicaVersion and
// ResizeProposal.Preflight all run byte-for-byte as they do against mainnet.
// The subnet is built from in-test principals, not any committed proposal
// config.
func TestPreflightAgainstSeededSubnet(t *testing.T) {
	const seededVersion = "0000000000000000000000000000000000000001"
	seeds := []SubnetSeed{{SubnetID: subnetX, NumNodes: 3, ReplicaVersion: seededVersion}}

	n, _, url := startNNSSeeded(t, principal.Principal{}, seeds, true)
	members := n.Members[subnetX.Encode()]
	if len(members) != 3 {
		t.Fatalf("seed should have generated 3 members, got %d", len(members))
	}

	got, err := FetchSubnetMembership(url, true, subnetX, DisableQueryVerification())
	if err != nil {
		t.Fatalf("fetch membership over gateway: %v", err)
	}
	want := sortedEncode(members)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("membership mismatch:\n got %v\nwant %v", got, want)
	}

	ver, err := FetchSubnetReplicaVersion(url, true, subnetX, DisableQueryVerification())
	if err != nil {
		t.Fatalf("fetch replica version: %v", err)
	}
	if ver != seededVersion {
		t.Errorf("replica version = %q, want %q", ver, seededVersion)
	}

	// Remove a real member and add nodeC (not a member): a real change, so
	// Preflight must not flag it (Clean, empty report).
	real := ResizeProposal{
		SubnetID:      subnetX,
		NodeIDsAdd:    []principal.Principal{nodeC},
		NodeIDsRemove: []principal.Principal{members[0]},
	}
	pf, err := real.Preflight(url, true, DisableQueryVerification())
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if pf.Level != PreflightClean {
		t.Errorf("a real add+remove is not a no-op; want Clean, got %v\n%s", pf.Level, pf.Report)
	}

	// Remove nodeC, which is not a member: a phantom no-op, and the only op, so
	// the whole plan is a no-op -> NoOp.
	phantom := ResizeProposal{SubnetID: subnetX, NodeIDsRemove: []principal.Principal{nodeC}}
	pf, err = phantom.Preflight(url, true, DisableQueryVerification())
	if err != nil {
		t.Fatalf("phantom preflight: %v", err)
	}
	if pf.Level != PreflightNoOp {
		t.Errorf("phantom-only remove should be NoOp, got %v\n%s", pf.Level, pf.Report)
	}

	// deploy_guestos preflight over the same gateway: the seeded version is a
	// no-op, a different elected one is a real upgrade, an unelected one is
	// refused. Both elected versions are stubbed to match the local registry.
	const electedVersion = "0000000000000000000000000000000000000002"
	src := stubSources(t, seededVersion, electedVersion)
	deployOpts := append([]FetchOption{DisableQueryVerification()}, src...)

	sameVer := DeployGuestosAction{SubnetID: subnetX, ReplicaVersionID: seededVersion}
	pf, err = sameVer.Preflight(url, true, deployOpts...)
	if err != nil {
		t.Fatalf("deploy_guestos preflight (same version): %v", err)
	}
	if pf.Level != PreflightNoOp {
		t.Errorf("deploying the seeded version should be NoOp, got %v\n%s", pf.Level, pf.Report)
	}
	newVer := DeployGuestosAction{SubnetID: subnetX, ReplicaVersionID: electedVersion}
	pf, err = newVer.Preflight(url, true, deployOpts...)
	if err != nil {
		t.Fatalf("deploy_guestos preflight (new version): %v", err)
	}
	if pf.Level != PreflightClean {
		t.Errorf("deploying a different elected version is a real upgrade (Clean), got %v\n%s", pf.Level, pf.Report)
	}
	unelected := DeployGuestosAction{SubnetID: subnetX, ReplicaVersionID: "000000000000000000000000000000000000dead"}
	pf, err = unelected.Preflight(url, true, deployOpts...)
	if err != nil {
		t.Fatalf("deploy_guestos preflight (unelected): %v", err)
	}
	if pf.Level != PreflightNoOp {
		t.Errorf("an unelected version must be refused, got %v\n%s", pf.Level, pf.Report)
	}

	// A subnet the registry does not know (nodeC is not a seeded subnet) must
	// surface an error, not empty data.
	if _, err := FetchSubnetMembership(url, true, nodeC, DisableQueryVerification()); err == nil {
		t.Error("fetching an unknown subnet's membership should error")
	}
	if _, err := FetchSubnetReplicaVersion(url, true, nodeC, DisableQueryVerification()); err == nil {
		t.Error("fetching an unknown subnet's replica version should error")
	}
}

// TestReconcileAgainstSeededSubnet drives Reconcile over the same seeded-registry
// gateway the Preflight e2e uses: one live member declared (in-sync), one node
// declared on the subnet that is not a live member (missing), and the remaining
// live members left undeclared (unmanaged).
func TestReconcileAgainstSeededSubnet(t *testing.T) {
	seeds := []SubnetSeed{{SubnetID: subnetX, NumNodes: 3, ReplicaVersion: "0000000000000000000000000000000000000001"}}
	n, _, url := startNNSSeeded(t, principal.Principal{}, seeds, true)
	members := n.Members[subnetX.Encode()]
	if len(members) != 3 {
		t.Fatalf("seed should have generated 3 members, got %d", len(members))
	}

	live, err := FetchSubnetMembership(url, true, subnetX, DisableQueryVerification())
	if err != nil {
		t.Fatalf("fetch membership over gateway: %v", err)
	}

	sub := subnetX.Encode()
	r := res(
		[]NodeRes{
			{Name: "declared_live", ID: members[0].Encode(), Subnet: sub},
			{Name: "declared_gone", ID: nodeC.Encode(), Subnet: sub},
		},
		[]Subnet{{Name: "x", ID: sub, Label: "Subnet X"}},
	)
	rc := Reconcile(r, sub, live, nil)

	if got := rowFor(rc, members[0].Encode()); got == nil || got.Status != ReconcileInSync {
		t.Errorf("member[0]: got %+v, want in-sync", got)
	}
	if got := rowFor(rc, nodeC.Encode()); got == nil || got.Status != ReconcileMissing {
		t.Errorf("nodeC: got %+v, want missing", got)
	}
	for _, m := range members[1:] {
		if got := rowFor(rc, m.Encode()); got == nil || got.Status != ReconcileUnmanaged {
			t.Errorf("undeclared member %s: got %+v, want unmanaged", m.Encode(), got)
		}
	}
	if !rc.HasDrift() {
		t.Error("expected drift")
	}
}

// TestReconcileProvidersAgainstSeededRegistry seeds a provider/operator/dc set
// and exercises the real query path: FetchProviderOperators over the gateway,
// then ReconcileProviders classifying a matching operator (ok), a dc mismatch,
// and an operator the provider does not own (unknown).
func TestReconcileProvidersAgainstSeededRegistry(t *testing.T) {
	const (
		provID = "mrfhx-rsvqz-jndwd-3nrkb-fw3wy-cq64z-iszxt-drffc-f4rtj-ivoop-6ae"
		opID   = "u7afs-z2fqh-zbqyo-jufwe-3vqqs-chc7f-k2fe4-rt66w-l4qia-keuuj-qqe"
	)
	seeds := []SubnetSeed{{SubnetID: subnetX, NumNodes: 1, ReplicaVersion: "0000000000000000000000000000000000000001"}}
	providers := []ProviderSeed{{ProviderID: provID, OperatorID: opID, DcID: "vd1", DcRegion: "Europe,CH,Vaud"}}
	_, _, url := startNNSSeededWithProviders(t, principal.Principal{}, seeds, providers, true)

	pid := principal.MustDecode(provID)
	ops, err := FetchProviderOperators(url, true, pid, DisableQueryVerification())
	if err != nil {
		t.Fatalf("fetch provider operators: %v", err)
	}
	if len(ops) != 1 || ops[0].OperatorID != opID || ops[0].DcID != "vd1" || ops[0].DcRegion != "Europe,CH,Vaud" {
		t.Fatalf("unexpected operators: %+v", ops)
	}

	r := &Resources{
		Providers: []NodeProvider{{Name: "p", ID: provID, Label: ""}},
		DCs:       []DataCenter{{Name: "vd1", ID: "vd1", Label: "", Region: "Europe,CH,Vaud"}},
		Operators: []NodeOperator{
			{Name: "good", ID: opID, Label: "", Provider: provID, Dc: "vd1"},
			{Name: "bad_dc", ID: opID, Label: "", Provider: provID, Dc: "so1"},
			{Name: "phantom", ID: "aaaaa-aa", Label: "", Provider: provID, Dc: "vd1"},
		},
		labels: map[string]string{},
	}
	pr := ReconcileProviders(r, map[string][]ProviderOperator{provID: ops})

	if got := opRowFor(pr, opID); got == nil || got.Status != OperatorOK {
		t.Errorf("good operator: got %+v, want ok", got)
	}
	if got := opRowFor(pr, "aaaaa-aa"); got == nil || got.Status != OperatorUnknown {
		t.Errorf("phantom operator: got %+v, want unknown", got)
	}
}

func TestSubmitViaHotkey(t *testing.T) {
	hotkeyID, err := NewIdentity()
	if err != nil {
		t.Fatalf("generate hotkey identity: %v", err)
	}
	n := startNNS(t, hotkeyID.Principal())
	if len(n.Hotkey.Raw) == 0 {
		t.Fatal("hotkey not set on NNS")
	}

	// Submit as the hotkey principal, not the controller.
	pid, err := n.SubmitResizeAs(n.Hotkey, n.ProposerNeuron(), resizeFixture(t))
	if err != nil {
		t.Fatalf("submit via hotkey: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	rendered := Render(pi)
	t.Logf("\n%s", rendered)
	assertResizeRendered(t, rendered)
}
