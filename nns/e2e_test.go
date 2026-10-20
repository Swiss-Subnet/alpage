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
	"time"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
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
	return startNNSWith(t, hotkey, DryRunState{Subnets: seeds, Providers: providers}, live)
}

// startNNSWith brings the NNS up from a DryRunState, the same value a kind's
// DryRunSeeds returns, so a test can plant exactly what apply would.
func startNNSWith(t *testing.T, hotkey principal.Principal, state DryRunState, live bool) (*NNS, *pocketic.Client, string) {
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
	n, err := BringUpWith(c, inst, testController(), BringUpOptions{
		Hotkey:      hotkey,
		Subnets:     state.Subnets,
		Providers:   state.Providers,
		DataCenters: state.DataCenters,
	})
	if err != nil {
		t.Fatalf("bring up: %v", err)
	}
	if err := c.SetTime(inst, 1_700_000_000_000_000_000); err != nil {
		t.Fatalf("set_time: %v", err)
	}
	if err := c.AutoProgress(inst); err != nil {
		t.Fatalf("auto_progress: %v", err)
	}
	for range 5 {
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

// membershipFixture loads the self-contained membership fixture (the live config lives
// in a separate repo).
func membershipFixture(t *testing.T) MembershipProposal {
	t.Helper()
	spec, err := LoadSpec("testdata/golden_src/proposals.hcl", "membership-fixture")
	if err != nil {
		t.Fatalf("load membership-fixture spec: %v", err)
	}
	p, err := spec.Proposal()
	if err != nil {
		t.Fatalf("decode membership-fixture spec: %v", err)
	}
	return p
}

func assertMembershipRendered(t *testing.T, rendered string) {
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

func TestSubmitMembershipFixture(t *testing.T) {
	n := startNNS(t, principal.Principal{})
	neuron := n.ProposerNeuron()

	pid, err := n.SubmitMembership(neuron, membershipFixture(t))
	if err != nil {
		t.Fatalf("submit membership: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	rendered := Render(pi)
	t.Logf("\n%s", rendered)

	assertMembershipRendered(t, rendered)
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

// Governance's terminal proposal statuses. A proposal that reaches Failed was
// adopted and then trapped in the target canister, which is the failure mode
// alpage's preflights and dry-run exist to catch before the vote.
const (
	proposalExecuted = int32(governancepb.ProposalStatus_PROPOSAL_STATUS_EXECUTED)
	proposalFailed   = int32(governancepb.ProposalStatus_PROPOSAL_STATUS_FAILED)
)

// awaitTerminal polls a submitted proposal until its execution has either
// succeeded or failed. Adoption is not what is being waited on, since the seeded
// neuron holds the whole voting power and a proposal is adopted on submission.
// Governance's inter-canister call into the registry is, and it takes a few
// rounds; the instance runs with auto_progress so those rounds advance on their
// own.
func awaitTerminal(t *testing.T, n *NNS, pid uint64) *governance.ProposalInfo {
	t.Helper()
	for range 60 {
		pi, err := n.GetProposalInfo(pid)
		if err != nil {
			t.Fatalf("get proposal info: %v", err)
		}
		if pi.Status == proposalExecuted || pi.Status == proposalFailed {
			return pi
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("proposal %d neither executed nor failed", pid)
	return nil
}

// awaitExecuted is awaitTerminal for the proposals a test expects to land.
func awaitExecuted(t *testing.T, n *NNS, pid uint64) *governance.ProposalInfo {
	t.Helper()
	pi := awaitTerminal(t, n, pid)
	if pi.Status != proposalExecuted {
		t.Fatalf("proposal %d failed to execute: %+v", pid, pi.FailureReason)
	}
	return pi
}

// TestUpdateSubnetExecutesAgainstSeededSubnet is the end-to-end check for the
// claim the whole kind rests on: the registry merges this payload field by
// field, so a proposal changes what it declares and leaves everything else
// alone. Reading the Rust says so; only executing two proposals against a real
// registry canister proves it.
//
// It also pins the NNS function number the way the deploy_guestos e2e does:
// governance stores the number alpage sent, so reading #7 back is ground truth
// no in-repo constant can fake.
func TestUpdateSubnetExecutesAgainstSeededSubnet(t *testing.T) {
	seeds := []SubnetSeed{{SubnetID: subnetX, NumNodes: 3, ReplicaVersion: "0000000000000000000000000000000000000001"}}
	n, _, url := startNNSSeeded(t, principal.Principal{}, seeds, true)
	neuron := n.ProposerNeuron()
	read := []FetchOption{DisableQueryVerification()}

	// Preflight over the gateway, against the seeded record: the subnet is not
	// halted, so halting it is a real change.
	halted := true
	pf, err := UpdateSubnetAction{SubnetID: subnetX, Settings: SubnetSettings{IsHalted: &halted}}.
		Preflight(url, true, read...)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if pf.Level != PreflightClean {
		t.Errorf("halting an unhalted subnet should be Clean, got %v\n%s", pf.Level, pf.Report)
	}

	// First proposal: one field, which must land.
	cap42 := uint64(42)
	pid, err := n.SubmitAs(n.Proposer, neuron, UpdateSubnetAction{
		Meta:     Meta{Title: "cap canisters", Summary: "e2e"},
		SubnetID: subnetX,
		Settings: SubnetSettings{MaxNumberOfCanisters: &cap42},
	})
	if err != nil {
		t.Fatalf("submit update_subnet: %v", err)
	}
	rendered := RenderVerbose(awaitExecuted(t, n, pid))
	t.Logf("\n%s", rendered)
	for _, want := range []string{
		"ExecuteNnsFunction #7",
		"update_subnet",
		"max_number_of_canisters: 42",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered proposal missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "is_halted") {
		t.Errorf("rendered proposal shows is_halted, which it does not declare:\n%s", rendered)
	}

	rec, err := getSubnetRecord(url, true, subnetX, read)
	if err != nil {
		t.Fatalf("read subnet record: %v", err)
	}
	if rec.MaxNumberOfCanisters != 42 {
		t.Fatalf("max_number_of_canisters = %d, want 42", rec.MaxNumberOfCanisters)
	}

	// Second proposal: a different single field. If the registry replaced the
	// record rather than merging it, the cap set above would be back to 0 and
	// every "omitted means untouched" claim in the docs would be wrong.
	pid, err = n.SubmitAs(n.Proposer, neuron, UpdateSubnetAction{
		Meta:     Meta{Title: "halt subnet", Summary: "e2e"},
		SubnetID: subnetX,
		Settings: SubnetSettings{IsHalted: &halted},
	})
	if err != nil {
		t.Fatalf("submit update_subnet (halt): %v", err)
	}
	awaitExecuted(t, n, pid)

	rec, err = getSubnetRecord(url, true, subnetX, read)
	if err != nil {
		t.Fatalf("read subnet record after halt: %v", err)
	}
	if !rec.IsHalted {
		t.Error("is_halted did not take effect")
	}
	if rec.MaxNumberOfCanisters != 42 {
		t.Errorf("max_number_of_canisters = %d after a proposal that did not declare it; want 42 (untouched)", rec.MaxNumberOfCanisters)
	}
	if got := len(rec.Membership); got != 3 {
		t.Errorf("membership has %d nodes after update_subnet; want the seeded 3 untouched", got)
	}

	// The now-halted, capped subnet is what preflight should read back: the same
	// halt is a no-op, which apply refuses without --force.
	pf, err = UpdateSubnetAction{SubnetID: subnetX, Settings: SubnetSettings{IsHalted: &halted}}.
		Preflight(url, true, read...)
	if err != nil {
		t.Fatalf("preflight after halt: %v", err)
	}
	if pf.Level != PreflightNoOp {
		t.Errorf("halting an already-halted subnet should be NoOp, got %v\n%s", pf.Level, pf.Report)
	}

	// Same instance, third proposal: a change that executes and then traps on an
	// invariant. The registry allows enabling SEV, but check_sev_subnet_invariants
	// then requires every node to carry a chip ID, and the seeded nodes have none.
	// It must end in Failed. That is the post-adoption trap the dry-run surfaces,
	// and the reason a proposal that looks well-formed cannot be trusted without
	// one.
	on := SubnetFeatureSet{SevEnabled: true}
	pid, err = n.SubmitAs(n.Proposer, neuron, UpdateSubnetAction{
		Meta:     Meta{Title: "enable sev", Summary: "e2e"},
		SubnetID: subnetX,
		Settings: SubnetSettings{Features: &on},
	})
	if err != nil {
		t.Fatalf("submit update_subnet (sev): %v", err)
	}
	pi := awaitTerminal(t, n, pid)
	if pi.Status != proposalFailed {
		t.Fatalf("enabling sev_enabled reached status %d; the registry was expected to reject it", pi.Status)
	}
	if pi.FailureReason == nil || !strings.Contains(pi.FailureReason.ErrorMessage, "chip ID") {
		t.Errorf("execution failed for the wrong reason: %+v", pi.FailureReason)
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
// MembershipProposal.Preflight all run byte-for-byte as they do against mainnet.
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
	real := MembershipProposal{
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
	phantom := MembershipProposal{SubnetID: subnetX, NodeIDsRemove: []principal.Principal{nodeC}}
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
	pid, err := n.SubmitMembershipAs(n.Hotkey, n.ProposerNeuron(), membershipFixture(t))
	if err != nil {
		t.Fatalf("submit via hotkey: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	rendered := Render(pi)
	t.Logf("\n%s", rendered)
	assertMembershipRendered(t, rendered)
}

// TestSummaryRoundTripsThroughGovernance is the ground truth behind the dry
// run's summary output. A multi-line body reaching governance intact is what
// makes printing it back evidence rather than an echo of the config, and the
// 30000 byte cap alpage enforces at load is only worth enforcing if it is the
// cap the canister enforces.
func TestSummaryRoundTripsThroughGovernance(t *testing.T) {
	n := startNNS(t, principal.Principal{})
	neuron := n.ProposerNeuron()

	body := "First line.\n\n" + strings.Repeat("Body that a heredoc would carry. ", 40) + "\nLast line.\n"
	d := DeployGuestosAction{
		Meta:             Meta{Title: "summary round trip", Summary: body},
		SubnetID:         subnetX,
		ReplicaVersionID: "0000000000000000000000000000000000000001",
	}
	pid, err := n.SubmitAs(n.Proposer, neuron, d)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	if pi.Proposal == nil || pi.Proposal.Summary != body {
		t.Errorf("governance stored a different summary:\n%q", pi.Proposal.Summary)
	}
	if rendered := RenderVerbose(pi); !strings.Contains(rendered, "Last line.") {
		t.Errorf("the verbose render dropped the end of the body:\n%s", rendered)
	}

	over := d
	over.Summary = strings.Repeat("s", proposalSummaryMax+1)
	if _, err := n.SubmitAs(n.Proposer, neuron, over); err == nil {
		t.Errorf("governance accepted %d bytes, so the cap alpage enforces at load is wrong", len(over.Summary))
	}
}
