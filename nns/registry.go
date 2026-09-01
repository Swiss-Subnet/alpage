package nns

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/aviate-labs/agent-go"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/swiss-subnet/alpage/gen/registry"
	"github.com/zclconf/go-cty/cty"
)

// FetchOption tunes how the read-only registry fetchers talk to the network.
type FetchOption func(*fetchOpts)

type fetchOpts struct {
	disableQueryVerify bool
	explorerBase       string
	dashboardBase      string
	allowUnverified    bool
	subnets            []principal.Principal
}

// WithSubnets supplies the subnets to check node membership against. The
// registry has no node->subnet index, so remove_nodes preflight can only look
// where it is told; callers pass the subnets resources.hcl declares.
func WithSubnets(ids ...principal.Principal) FetchOption {
	return func(o *fetchOpts) { o.subnets = append(o.subnets, ids...) }
}

// AllowUnverifiedElection lets deploy_guestos preflight proceed when the
// elected-version set cannot be read, downgrading the hard failure to a
// warning. For the case where the source is down and the deploy must go ahead;
// the proposal may still be rejected by the NNS.
func AllowUnverifiedElection() FetchOption {
	return func(o *fetchOpts) { o.allowUnverified = true }
}

// explorer returns the registry explorer base to query, defaulting to the
// public one.
func (o fetchOpts) explorer() string {
	if o.explorerBase != "" {
		return o.explorerBase
	}
	return DefaultRegistryExplorer
}

// dashboard returns the dashboard proposals endpoint to query, defaulting to
// the public one.
func (o fetchOpts) dashboard() string {
	if o.dashboardBase != "" {
		return o.dashboardBase
	}
	return DashboardAPI
}

// WithExplorer overrides the registry explorer base URL. Tests point this at a
// local stub so preflight does not reach the public internet.
func WithExplorer(base string) FetchOption {
	return func(o *fetchOpts) { o.explorerBase = base }
}

// WithDashboard overrides the dashboard proposals endpoint. Tests point this at
// a local stub.
func WithDashboard(base string) FetchOption {
	return func(o *fetchOpts) { o.dashboardBase = base }
}

// DisableQueryVerification turns off signed-query verification. Only for tests
// against a local replica (e.g. PocketIC), whose seeded nodes are not in the
// certified state tree the agent would consult; never pass it for mainnet.
func DisableQueryVerification() FetchOption {
	return func(o *fetchOpts) { o.disableQueryVerify = true }
}

func newRegistryAgent(host string, fetchRootKey bool, opts []FetchOption) (*registry.RegistryAgent, error) {
	if host == "" {
		host = MainnetHost
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse host %q: %w", host, err)
	}
	var o fetchOpts
	for _, opt := range opts {
		opt(&o)
	}
	return registry.NewRegistryAgent(RegistryID, agent.Config{
		ClientConfig:                   clientOptions(u),
		FetchRootKey:                   fetchRootKey,
		DisableSignedQueryVerification: o.disableQueryVerify,
	})
}

// getSubnetRecord runs get_subnet and unwraps the Ok arm, so the callers that
// only differ in which field they read do not each repeat the error ladder.
func getSubnetRecord(host string, fetchRootKey bool, subnetID principal.Principal, opts []FetchOption) (*registry.SubnetRecord, error) {
	a, err := newRegistryAgent(host, fetchRootKey, opts)
	if err != nil {
		return nil, fmt.Errorf("new registry agent: %w", err)
	}
	resp, err := a.GetSubnet(registry.GetSubnetRequest{SubnetId: &subnetID})
	if err != nil {
		return nil, fmt.Errorf("get_subnet: %w", err)
	}
	if resp.Err != nil {
		return nil, fmt.Errorf("registry rejected get_subnet: %s", *resp.Err)
	}
	if resp.Ok == nil {
		return nil, fmt.Errorf("get_subnet: empty response")
	}
	return resp.Ok, nil
}

// FetchSubnetMembership returns a subnet's node ids as sorted textual
// principals. Read-only query: no identity, host defaults to mainnet.
func FetchSubnetMembership(host string, fetchRootKey bool, subnetID principal.Principal, opts ...FetchOption) ([]string, error) {
	rec, err := getSubnetRecord(host, fetchRootKey, subnetID, opts)
	if err != nil {
		return nil, err
	}
	return decodeMembership(rec.Membership), nil
}

// FetchSubnetRecordFacts returns the subnet record fields reconcile compares
// against: type, cost schedule, and admins. Read-only.
func FetchSubnetRecordFacts(host string, fetchRootKey bool, subnetID principal.Principal, opts ...FetchOption) (SubnetRecordFacts, error) {
	rec, err := getSubnetRecord(host, fetchRootKey, subnetID, opts)
	if err != nil {
		return SubnetRecordFacts{}, err
	}
	admins := make([]string, 0, len(rec.SubnetAdmins))
	for _, p := range rec.SubnetAdmins {
		admins = append(admins, p.Encode())
	}
	return SubnetRecordFacts{
		Type:         candidSubnetTypeName(rec.SubnetType),
		CostSchedule: candidCostScheduleName(rec.CanisterCyclesCostSchedule),
		Admins:       admins,
		Features:     SubnetFeatures{SevEnabled: rec.Features.SevEnabled},
	}, nil
}

// candidSubnetTypeName maps the Candid SubnetType variant to its HCL name. The
// variant carries one non-nil arm; an all-nil value means the record omitted the
// field, which reads as the default.
func candidSubnetTypeName(t registry.SubnetType) string {
	switch {
	case t.CloudEngine != nil:
		return SubnetTypeCloudEngine
	case t.System != nil:
		return SubnetTypeSystem
	case t.VerifiedApplication != nil:
		return SubnetTypeVerifiedApplication
	case t.Application != nil:
		return SubnetTypeApplication
	}
	return ""
}

func candidCostScheduleName(s registry.CanisterCyclesCostSchedule) string {
	switch {
	case s.Free != nil:
		return CostScheduleFree
	case s.Normal != nil:
		return CostScheduleNormal
	}
	return ""
}

// FetchSubnetReplicaVersion returns a subnet's current GuestOS/replica version
// id from the registry. Read-only.
func FetchSubnetReplicaVersion(host string, fetchRootKey bool, subnetID principal.Principal, opts ...FetchOption) (string, error) {
	rec, err := getSubnetRecord(host, fetchRootKey, subnetID, opts)
	if err != nil {
		return "", err
	}
	return rec.ReplicaVersionId, nil
}

// ProviderOperator is one (operator, data center) pair owned by a node
// provider, as returned by the registry's
// get_node_operators_and_dcs_of_node_provider query.
type ProviderOperator struct {
	OperatorID string
	DcID       string
	DcRegion   string
}

// FetchProviderOperators returns the operators (and their data centers) the
// registry records for a node provider. Read-only, trustless: a typed query on
// the registry canister, no HTTP explorer. An empty slice means the provider
// owns no operators; it does NOT mean the provider is absent, since the registry
// answers Ok with an empty vector for an unregistered principal too.
func FetchProviderOperators(host string, fetchRootKey bool, providerID principal.Principal, opts ...FetchOption) ([]ProviderOperator, error) {
	a, err := newRegistryAgent(host, fetchRootKey, opts)
	if err != nil {
		return nil, fmt.Errorf("new registry agent: %w", err)
	}
	resp, err := a.GetNodeOperatorsAndDcsOfNodeProvider(providerID)
	if err != nil {
		return nil, fmt.Errorf("get_node_operators_and_dcs_of_node_provider: %w", err)
	}
	if resp.Err != nil {
		return nil, fmt.Errorf("registry rejected query: %s", *resp.Err)
	}
	if resp.Ok == nil {
		return nil, nil
	}
	out := make([]ProviderOperator, 0, len(*resp.Ok))
	for _, pair := range *resp.Ok {
		out = append(out, ProviderOperator{
			OperatorID: principal.Principal{Raw: pair.Field1.NodeOperatorPrincipalId}.String(),
			DcID:       pair.Field1.DcId,
			DcRegion:   pair.Field0.Region,
		})
	}
	return out, nil
}

// RenderResourcesHCL renders a subnet and its node membership as a
// resources.hcl fragment, nodes sorted by id.
func RenderResourcesHCL(subnetID string, nodeIDs []string) string {
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	sn := body.AppendNewBlock("subnet", []string{blockLabel("subnet", subnetID)})
	sn.Body().SetAttributeValue("id", cty.StringVal(subnetID))

	sorted := append([]string(nil), nodeIDs...)
	sort.Strings(sorted)
	for _, id := range sorted {
		body.AppendNewline()
		n := body.AppendNewBlock("node", []string{blockLabel("node", id)})
		n.Body().SetAttributeValue("id", cty.StringVal(id))
	}
	return string(f.Bytes())
}

// blockLabel derives a valid HCL identifier from a principal: an id verbatim is
// not one (leading digit, hyphens).
func blockLabel(kind, id string) string {
	prefix := id
	if i := strings.IndexByte(id, '-'); i > 0 {
		prefix = id[:i]
	}
	return kind + "_" + prefix
}

// decodeMembership turns raw node-id principals into sorted textual principals.
func decodeMembership(raw [][]byte) []string {
	out := make([]string, 0, len(raw))
	for _, b := range raw {
		out = append(out, principal.Principal{Raw: b}.String())
	}
	sort.Strings(out)
	return out
}
