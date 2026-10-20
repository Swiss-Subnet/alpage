package nns

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
	"github.com/swiss-subnet/alpage/gen/registry"
	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
)

// The onboarding counterparts of removal.go. The data center and the provider
// are independent of each other and can go in either order or at once. Only
// the operator depends on both, since it names them. Nothing in the payloads
// encodes that, so Preflight enforces it.
//
// Registering a node is not here. The operator calls add_node directly,
// spending the allowance granted below.

// --- add_data_centers: AddOrRemoveDataCenters ---

// DataCenterSpec is one data center record to register.
type DataCenterSpec struct {
	ID     string
	Region string
	Owner  string
	GPS    *GPSCoordinates
}

// GPSCoordinates is float32 because that is what the registry record holds.
type GPSCoordinates struct {
	Latitude  float32
	Longitude float32
}

// AddDataCentersAction registers data center records. The NNS function is
// add_or_remove_data_centers, but this kind only adds. Removal has its own
// preconditions and belongs in its own kind.
type AddDataCentersAction struct {
	Meta
	DataCenters []DataCenterSpec
}

func (a AddDataCentersAction) Kind() string { return "add_data_centers" }
func (a AddDataCentersAction) NnsFunction() governancepb.NnsFunction {
	return governancepb.NnsFunction_NNS_FUNCTION_ADD_OR_REMOVE_DATA_CENTERS
}

func (a AddDataCentersAction) PayloadBlob() ([]byte, error) {
	add := make([]registry.DataCenterRecord, len(a.DataCenters))
	for i, dc := range a.DataCenters {
		add[i] = registry.DataCenterRecord{
			Id:     dc.ID,
			Region: dc.Region,
			Owner:  dc.Owner,
			Gps:    dc.GPS.candid(),
		}
	}
	payload := registry.AddOrRemoveDataCentersProposalPayload{
		DataCentersToAdd:    add,
		DataCentersToRemove: make([]string, 0),
	}
	blob, err := candid.Marshal([]any{payload})
	if err != nil {
		return nil, fmt.Errorf("encode AddOrRemoveDataCentersProposalPayload: %w", err)
	}
	return blob, nil
}

func (a AddDataCentersAction) ProposalAction() (*governance.ProposalActionRequest, error) {
	return execNnsAction(a)
}

// DryRunSeeds asks only for the system subnet the init invariants want.
// Seeding the data center we are adding would make the dry run upsert over it.
func (a AddDataCentersAction) DryRunSeeds([]string) DryRunState {
	return DryRunState{Subnets: systemSubnetSeeds()}
}

// Preflight reads each target back. The mutation is an upsert, so an existing
// record is silently overwritten rather than rejected.
func (a AddDataCentersAction) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	ids := make([]string, len(a.DataCenters))
	for i, dc := range a.DataCenters {
		ids[i] = dc.ID
	}
	current, err := FetchDataCenters(host, fetchRootKey, ids)
	if err != nil {
		return Preflight{}, fmt.Errorf("fetch data centers: %w", err)
	}
	return preconditionPreflight(planAddDataCenters(a.DataCenters, current)), nil
}

func (a AddDataCentersAction) RenderPayload(b *strings.Builder, blob []byte) {
	var p registry.AddOrRemoveDataCentersProposalPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	fmt.Fprintf(b, "    data centers added (%d):\n", len(p.DataCentersToAdd))
	for _, dc := range p.DataCentersToAdd {
		fmt.Fprintf(b, "      + %s\n", dc.Id)
		fmt.Fprintf(b, "          region: %s\n", dc.Region)
		fmt.Fprintf(b, "          owner:  %s\n", dc.Owner)
		if dc.Gps != nil {
			fmt.Fprintf(b, "          gps:    %g, %g\n", dc.Gps.Latitude, dc.Gps.Longitude)
		}
	}
}

func (g *GPSCoordinates) candid() *registry.Gps {
	if g == nil {
		return nil
	}
	return &registry.Gps{Latitude: g.Latitude, Longitude: g.Longitude}
}

// --- add_node_provider: AddOrRemoveNodeProvider (native governance action) ---

// AddNodeProviderAction registers a node provider. Like its removal twin this
// is a native governance action, so it carries no NNS function number. The
// self-declaration is not part of the payload. It goes in summary and url.
type AddNodeProviderAction struct {
	Meta
	ProviderID principal.Principal
	// RewardAccount is the 32 raw bytes of the ledger account identifier. Nil
	// leaves it unset, which pays rewards to the default account of the
	// provider principal.
	RewardAccount []byte
}

func (a AddNodeProviderAction) Kind() string { return "add_node_provider" }

func (a AddNodeProviderAction) NnsFunction() governancepb.NnsFunction {
	return governancepb.NnsFunction_NNS_FUNCTION_UNSPECIFIED
}

// PayloadBlob encodes the change so state can hash it. ProposalAction is what
// gets submitted.
func (a AddNodeProviderAction) PayloadBlob() ([]byte, error) {
	blob, err := candid.Marshal([]any{a.change()})
	if err != nil {
		return nil, fmt.Errorf("encode AddOrRemoveNodeProvider change: %w", err)
	}
	return blob, nil
}

func (a AddNodeProviderAction) change() governance.Change {
	id := a.ProviderID
	np := governance.NodeProvider{Id: &id}
	if a.RewardAccount != nil {
		np.RewardAccount = &governance.AccountIdentifier{Hash: a.RewardAccount}
	}
	return governance.Change{ToAdd: &np}
}

func (a AddNodeProviderAction) ProposalAction() (*governance.ProposalActionRequest, error) {
	change := a.change()
	return &governance.ProposalActionRequest{
		AddOrRemoveNodeProvider: &governance.AddOrRemoveNodeProvider{Change: &change},
	}, nil
}

// DryRunSeeds seeds the target only if mainnet already has it, so the local
// duplicate check faces the same question mainnet will.
func (a AddNodeProviderAction) DryRunSeeds(mainnetProviders []string) DryRunState {
	want := a.ProviderID.Encode()
	for _, p := range mainnetProviders {
		if p != want {
			continue
		}
		return DryRunState{
			Providers: []ProviderSeed{{
				ProviderID: want,
				OperatorID: dryRunSeedOperator,
				DcID:       dryRunSeedDC,
				DcRegion:   dryRunSeedRegion,
			}},
			Subnets: systemSubnetSeeds(),
		}
	}
	return DryRunState{}
}

func (a AddNodeProviderAction) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	providers, err := FetchNodeProviders(host, fetchRootKey, opts...)
	if err != nil {
		return Preflight{}, fmt.Errorf("list node providers: %w", err)
	}
	id := a.ProviderID.Encode()
	return preconditionPreflight(planAddNodeProvider(id, slices.Contains(providers, id))), nil
}

func (a AddNodeProviderAction) RenderPayload(b *strings.Builder, blob []byte) {
	var c governance.Change
	if err := candid.Unmarshal(blob, []any{&c}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	renderProviderChange(b, c)
}

// renderProviderChange prints an AddOrRemoveNodeProvider change. Both provider
// kinds share it, and render() calls it for a proposal read back from
// governance, where the change arrives decoded rather than as a candid blob.
func renderProviderChange(b *strings.Builder, c governance.Change) {
	switch {
	case c.ToAdd != nil && c.ToAdd.Id != nil:
		fmt.Fprintf(b, "    node provider added:\n      + %s\n", c.ToAdd.Id.Encode())
		if ra := c.ToAdd.RewardAccount; ra != nil {
			fmt.Fprintf(b, "          reward_account: %x\n", ra.Hash)
		} else {
			acct := principal.NewAccountID(*c.ToAdd.Id, principal.DefaultSubAccount)
			fmt.Fprintf(b, "          reward_account: unset, so rewards go to the principal's default account %s\n", acct.Encode())
		}
	case c.ToRemove != nil && c.ToRemove.Id != nil:
		fmt.Fprintf(b, "    node provider removed:\n      - %s\n", c.ToRemove.Id.Encode())
	default:
		fmt.Fprintf(b, "    <no provider change>\n")
	}
}

// --- add_node_operator: AddNodeOperator (ASSIGN_NOID) ---

// AddNodeOperatorAction registers a node operator under a provider, in a data
// center, with an allowance of nodes it may register.
type AddNodeOperatorAction struct {
	Meta
	OperatorID principal.Principal
	ProviderID principal.Principal
	DcID       string
	// NodeAllowance of zero is legal. Cloud Engine operators do not need an
	// allowance at all. Any other operator cannot register a node until
	// update_node_operator_config raises it.
	NodeAllowance uint64
	IPv6          string
	// MaxRewardableNodes maps a node reward type to a cap. The payload's other
	// reward map, rewardable_nodes, is the field the canister itself calls the
	// old one, so alpage always sends it empty and does not expose it.
	MaxRewardableNodes map[string]uint32
}

func (a AddNodeOperatorAction) Kind() string { return "add_node_operator" }

// NnsFunction is ASSIGN_NOID, governance's name for the registry's
// add_node_operator method.
func (a AddNodeOperatorAction) NnsFunction() governancepb.NnsFunction {
	return governancepb.NnsFunction_NNS_FUNCTION_ASSIGN_NOID
}

// rewardTuple is the (type, count) pair the generated payload uses for both
// reward maps.
type rewardTuple = struct {
	Field0 string `ic:"0,tuple" json:"0"`
	Field1 uint32 `ic:"1,tuple" json:"1"`
}

func (a AddNodeOperatorAction) PayloadBlob() ([]byte, error) {
	operator, provider := a.OperatorID, a.ProviderID
	payload := registry.AddNodeOperatorPayload{
		NodeOperatorPrincipalId: &operator,
		NodeProviderPrincipalId: &provider,
		DcId:                    a.DcID,
		NodeAllowance:           a.NodeAllowance,
		RewardableNodes:         make([]rewardTuple, 0),
	}
	if a.IPv6 != "" {
		ipv6 := a.IPv6
		payload.Ipv6 = &ipv6
	}
	if a.MaxRewardableNodes != nil {
		max := rewardTuples(a.MaxRewardableNodes)
		payload.MaxRewardableNodes = &max
	}
	blob, err := candid.Marshal([]any{payload})
	if err != nil {
		return nil, fmt.Errorf("encode AddNodeOperatorPayload: %w", err)
	}
	return blob, nil
}

// rewardTuples sorts by reward type. Go map order is randomised, so an
// unsorted payload would hash differently on every reload and read as drift.
func rewardTuples(m map[string]uint32) []rewardTuple {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]rewardTuple, 0, len(keys))
	for _, k := range keys {
		out = append(out, rewardTuple{Field0: k, Field1: m[k]})
	}
	return out
}

func (a AddNodeOperatorAction) ProposalAction() (*governance.ProposalActionRequest, error) {
	return execNnsAction(a)
}

// DryRunSeeds plants the data center the node-operator invariant demands at
// execution time. It is seeded rather than mirrored so the dry run works
// before the add_data_centers proposal has landed. Whether the data center
// really exists on-chain is Preflight's question. The operator stays out of
// the seed, since a seeded one would make the add trap.
//
// The provider is seeded only when mainnet already has it. Governance rejects
// this proposal at submission with "The node provider specified in the payload
// is not registered", so an unregistered provider has to fail the dry run the
// same way rather than slip through to a real submission.
func (a AddNodeOperatorAction) DryRunSeeds(mainnetProviders []string) DryRunState {
	state := DryRunState{
		DataCenters: []DataCenterSeed{{ID: a.DcID, Region: dryRunSeedRegion}},
		Subnets:     systemSubnetSeeds(),
	}
	provider := a.ProviderID.Encode()
	if slices.Contains(mainnetProviders, provider) {
		state.Providers = []ProviderSeed{{
			ProviderID: provider,
			OperatorID: dryRunSeedOperator,
			DcID:       dryRunSeedDC,
			DcRegion:   dryRunSeedRegion,
		}}
	}
	return state
}

// Preflight checks the operator principal is free, its data center exists, and
// its provider is registered. The last two are why this proposal goes after
// the other two.
func (a AddNodeOperatorAction) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	dcExists, err := FetchDataCenterExists(host, fetchRootKey, a.DcID)
	if err != nil {
		return Preflight{}, fmt.Errorf("fetch data center %s: %w", a.DcID, err)
	}
	operatorExists, err := FetchNodeOperatorExists(host, fetchRootKey, a.OperatorID)
	if err != nil {
		return Preflight{}, fmt.Errorf("fetch node operator %s: %w", a.OperatorID.Encode(), err)
	}
	providers, err := FetchNodeProviders(host, fetchRootKey, opts...)
	if err != nil {
		return Preflight{}, fmt.Errorf("list node providers: %w", err)
	}
	providerID := a.ProviderID.Encode()
	return preconditionPreflight(planAddNodeOperator(
		a.OperatorID.Encode(), providerID, a.DcID,
		operatorExists, dcExists, slices.Contains(providers, providerID), a.NodeAllowance,
	)), nil
}

func (a AddNodeOperatorAction) RenderPayload(b *strings.Builder, blob []byte) {
	var p registry.AddNodeOperatorPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	fmt.Fprintf(b, "    node operator added:\n      + %s\n", principalOrUnset(p.NodeOperatorPrincipalId))
	fmt.Fprintf(b, "          node_provider:  %s\n", principalOrUnset(p.NodeProviderPrincipalId))
	fmt.Fprintf(b, "          dc_id:          %s\n", p.DcId)
	fmt.Fprintf(b, "          node_allowance: %d\n", p.NodeAllowance)
	if p.Ipv6 != nil {
		fmt.Fprintf(b, "          ipv6:           %s\n", *p.Ipv6)
	}
	if p.MaxRewardableNodes != nil {
		renderRewardTuples(b, "max_rewardable_nodes", *p.MaxRewardableNodes)
	} else {
		renderRewardTuples(b, "max_rewardable_nodes", nil)
	}
}

func renderRewardTuples(b *strings.Builder, name string, tuples []rewardTuple) {
	if len(tuples) == 0 {
		fmt.Fprintf(b, "          %s: none\n", name)
		return
	}
	fmt.Fprintf(b, "          %s:\n", name)
	for _, t := range tuples {
		fmt.Fprintf(b, "            %s: %d\n", t.Field0, t.Field1)
	}
}

func principalOrUnset(p *principal.Principal) string {
	if p == nil {
		return "<unset>"
	}
	return p.Encode()
}
