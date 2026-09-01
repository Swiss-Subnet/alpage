package nns

import (
	"fmt"
	"strings"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
	"github.com/swiss-subnet/alpage/gen/registry"
	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
)

// --- remove_nodes: RemoveNodes (deregister nodes) ---

// RemoveNodesAction deregisters nodes, freeing their operators' allowance.
// E.g. mainnet 142599. The registry canister panics on a node that is still a
// subnet member, so Preflight checks that before the proposal is cut.
type RemoveNodesAction struct {
	Meta
	NodeIDs []principal.Principal
}

func (r RemoveNodesAction) Kind() string { return "remove_nodes" }
func (r RemoveNodesAction) NnsFunction() governancepb.NnsFunction {
	return governancepb.NnsFunction_NNS_FUNCTION_REMOVE_NODES
}

func (r RemoveNodesAction) PayloadBlob() ([]byte, error) {
	blob, err := candid.Marshal([]any{registry.RemoveNodesPayload{NodeIds: r.NodeIDs}})
	if err != nil {
		return nil, fmt.Errorf("encode RemoveNodesPayload: %w", err)
	}
	return blob, nil
}

func (r RemoveNodesAction) ProposalAction() (*governance.ProposalActionRequest, error) {
	return execNnsAction(r)
}

// MirrorScope mirrors the nodes and, through them, their operators: removal
// credits the allowance back to the operator record, so an absent operator
// traps. The subnets are deliberately not mirrored -- a node still in a subnet
// is Preflight's check, and mirroring one here would make every removal of a
// current member trap locally.
func (r RemoveNodesAction) MirrorScope() MirrorScope {
	return MirrorScope{Nodes: r.NodeIDs}
}

// Preflight checks the precondition do_remove_nodes enforces by panicking: a
// node still in a subnet fails execution after the NNS has already adopted the
// proposal. Membership is read per declared subnet, since the registry has no
// node->subnet index.
func (r RemoveNodesAction) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	var o fetchOpts
	for _, opt := range opts {
		opt(&o)
	}
	inSubnet := map[string]string{}
	for _, sn := range o.subnets {
		members, err := FetchSubnetMembership(host, fetchRootKey, sn, opts...)
		if err != nil {
			return Preflight{}, fmt.Errorf("fetch membership for %s: %w", sn.Encode(), err)
		}
		for _, m := range members {
			inSubnet[m] = sn.Encode()
		}
	}
	registered := map[string]bool{}
	ids := make([]string, len(r.NodeIDs))
	for i, n := range r.NodeIDs {
		ids[i] = n.Encode()
		st, err := FetchNodeStatus(o.explorer(), ids[i])
		if err != nil {
			return Preflight{}, fmt.Errorf("fetch node record %s: %w", ids[i], err)
		}
		registered[ids[i]] = st.Registered
	}
	return removalPreflight(planRemoveNodes(ids, inSubnet, registered)), nil
}

func (r RemoveNodesAction) RenderPayload(b *strings.Builder, blob []byte) {
	var p registry.RemoveNodesPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	fmt.Fprintf(b, "    nodes removed (%d):\n", len(p.NodeIds))
	for _, n := range p.NodeIds {
		fmt.Fprintf(b, "      - %s\n", n.Encode())
	}
}

// --- remove_node_operators: RemoveNodeOperators ---

// RemoveNodeOperatorsAction deletes node operator records. E.g. mainnet 142603.
// The registry canister silently filters out operators that still own nodes, so
// the proposal would execute and report success while changing nothing;
// Preflight catches that.
type RemoveNodeOperatorsAction struct {
	Meta
	OperatorIDs []principal.Principal
}

func (r RemoveNodeOperatorsAction) Kind() string { return "remove_node_operators" }
func (r RemoveNodeOperatorsAction) NnsFunction() governancepb.NnsFunction {
	return governancepb.NnsFunction_NNS_FUNCTION_REMOVE_NODE_OPERATORS
}

// PayloadBlob fills node_operator_principals_to_remove. The payload also has a
// raw-bytes node_operators_to_remove field, which the canister panics on if
// both are set; the principal form is the current one.
func (r RemoveNodeOperatorsAction) PayloadBlob() ([]byte, error) {
	payload := registry.RemoveNodeOperatorsPayload{
		NodeOperatorsToRemove:          [][]byte{},
		NodeOperatorPrincipalsToRemove: &registry.NodeOperatorPrincipals{Principals: r.OperatorIDs},
	}
	blob, err := candid.Marshal([]any{payload})
	if err != nil {
		return nil, fmt.Errorf("encode RemoveNodeOperatorsPayload: %w", err)
	}
	return blob, nil
}

func (r RemoveNodeOperatorsAction) ProposalAction() (*governance.ProposalActionRequest, error) {
	return execNnsAction(r)
}

// MirrorScope mirrors the operator records the payload names. Their nodes are
// not mirrored: the registry has no operator->node index to walk, so whether an
// operator still owns nodes (which makes the removal a silent no-op) stays
// Preflight's job via the explorer.
func (r RemoveNodeOperatorsAction) MirrorScope() MirrorScope {
	return MirrorScope{Operators: r.OperatorIDs}
}

func (r RemoveNodeOperatorsAction) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	var o fetchOpts
	for _, opt := range opts {
		opt(&o)
	}
	owned := map[string][]string{}
	known := map[string]bool{}
	ids := make([]string, len(r.OperatorIDs))
	for i, op := range r.OperatorIDs {
		ids[i] = op.Encode()
		nodes, err := FetchOperatorNodes(o.explorer(), ids[i])
		if err != nil {
			return Preflight{}, fmt.Errorf("fetch operator nodes %s: %w", ids[i], err)
		}
		owned[ids[i]] = nodes
		// The explorer 404s an operator it does not know, which is also how an
		// operator with no nodes reads; treat a successful lookup as known.
		known[ids[i]] = true
	}
	return removalPreflight(planRemoveNodeOperators(ids, owned, known)), nil
}

func (r RemoveNodeOperatorsAction) RenderPayload(b *strings.Builder, blob []byte) {
	var p registry.RemoveNodeOperatorsPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	var ops []principal.Principal
	if p.NodeOperatorPrincipalsToRemove != nil {
		ops = p.NodeOperatorPrincipalsToRemove.Principals
	}
	fmt.Fprintf(b, "    node operators removed (%d):\n", len(ops))
	for _, op := range ops {
		fmt.Fprintf(b, "      - %s\n", op.Encode())
	}
}

// --- remove_node_provider: AddOrRemoveNodeProvider (native governance action) ---

// RemoveNodeProviderAction removes a node provider. E.g. mainnet 142930. Unlike
// every other kind here this is a native governance action rather than an
// ExecuteNnsFunction, so it carries no NNS function number.
type RemoveNodeProviderAction struct {
	Meta
	ProviderID principal.Principal
}

func (r RemoveNodeProviderAction) Kind() string { return "remove_node_provider" }

// NnsFunction is unspecified: a native action is not dispatched by function
// number. It is here only to satisfy Action, and is never encoded.
func (r RemoveNodeProviderAction) NnsFunction() governancepb.NnsFunction {
	return governancepb.NnsFunction_NNS_FUNCTION_UNSPECIFIED
}

// PayloadBlob encodes the change for hashing. It is not what gets submitted:
// ProposalAction carries the typed action instead. State still pins its hash,
// so a changed target is detected the same way as for any other kind.
func (r RemoveNodeProviderAction) PayloadBlob() ([]byte, error) {
	blob, err := candid.Marshal([]any{r.change()})
	if err != nil {
		return nil, fmt.Errorf("encode AddOrRemoveNodeProvider change: %w", err)
	}
	return blob, nil
}

func (r RemoveNodeProviderAction) change() governance.Change {
	id := r.ProviderID
	return governance.Change{ToRemove: &governance.NodeProvider{Id: &id}}
}

func (r RemoveNodeProviderAction) ProposalAction() (*governance.ProposalActionRequest, error) {
	change := r.change()
	return &governance.ProposalActionRequest{
		AddOrRemoveNodeProvider: &governance.AddOrRemoveNodeProvider{Change: &change},
	}, nil
}

// DryRunSeeds seeds the removal target only if mainnet governance reports it as
// registered. Seeding unconditionally would make the local ToRemove validation
// pass for any principal, so a wrong target would only surface after the vote.
//
// It returns a subnet seed alongside the provider: any registry init carrying
// mutations triggers the invariant battery, which requires subnet_list, an
// elected replica version, and a system subnet. Provider mutations alone trap
// with "no system subnet".
func (r RemoveNodeProviderAction) DryRunSeeds(mainnetProviders []string) ([]ProviderSeed, []SubnetSeed) {
	want := r.ProviderID.Encode()
	for _, p := range mainnetProviders {
		if p != want {
			continue
		}
		// The registry half of the provider seed is required even for a
		// governance-only check: seedProviderMutations joins an operator to a DC
		// record, and an incomplete pair fails the registry's invariants.
		provider := []ProviderSeed{{
			ProviderID: want,
			OperatorID: dryRunSeedOperator,
			DcID:       dryRunSeedDC,
			DcRegion:   dryRunSeedRegion,
		}}
		// Decode, not MustDecode: a bad constant must not panic the CLI mid-apply.
		sid, err := principal.Decode(dryRunSeedSubnet)
		if err != nil {
			return provider, nil
		}
		subnet := []SubnetSeed{{
			SubnetID:       sid,
			NumNodes:       1,
			ReplicaVersion: dryRunSeedVersion,
		}}
		return provider, subnet
	}
	return nil, nil
}

// Placeholder registry identifiers for the dry-run seed. The provider id is the
// only field the ToRemove validation reads; the rest exist to satisfy the
// registry's operator/DC and init invariants.
const (
	dryRunSeedOperator = "u7afs-z2fqh-zbqyo-jufwe-3vqqs-chc7f-k2fe4-rt66w-l4qia-keuuj-qqe"
	dryRunSeedDC       = "dr1"
	dryRunSeedRegion   = "Europe,CH,Zurich"
	dryRunSeedSubnet   = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"
	dryRunSeedVersion  = "0000000000000000000000000000000000000001"
)

// Preflight checks what governance validates before the vote (the provider is
// registered) plus what it does not: operators still pointing at the provider
// would be orphaned by the removal.
func (r RemoveNodeProviderAction) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	ops, err := FetchProviderOperators(host, fetchRootKey, r.ProviderID, opts...)
	if err != nil {
		return Preflight{}, fmt.Errorf("fetch provider operators: %w", err)
	}
	ids := make([]string, len(ops))
	for i, op := range ops {
		ids[i] = op.OperatorID
	}
	// The query answers for a provider the registry knows; an unknown one comes
	// back empty, which is indistinguishable from a provider with no operators.
	// Governance rejects the unknown case at validation, so the check that adds
	// information here is the orphaned-operator one.
	return removalPreflight(planRemoveNodeProvider(r.ProviderID.Encode(), true, ids)), nil
}

func (r RemoveNodeProviderAction) RenderPayload(b *strings.Builder, blob []byte) {
	var c governance.Change
	if err := candid.Unmarshal(blob, []any{&c}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	if c.ToRemove == nil || c.ToRemove.Id == nil {
		fmt.Fprintf(b, "    <no provider to remove>\n")
		return
	}
	fmt.Fprintf(b, "    node provider removed:\n      - %s\n", c.ToRemove.Id.Encode())
}
