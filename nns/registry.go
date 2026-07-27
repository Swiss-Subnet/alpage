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

type fetchOpts struct{ disableQueryVerify bool }

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

// FetchSubnetMembership returns a subnet's node ids as sorted textual
// principals. Read-only query: no identity, host defaults to mainnet.
func FetchSubnetMembership(host string, fetchRootKey bool, subnetID principal.Principal, opts ...FetchOption) ([]string, error) {
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
	return decodeMembership(resp.Ok.Membership), nil
}

// FetchSubnetFeatures returns a subnet's feature flags from its registry
// record. Read-only. A subnet record with no features block yields the zero
// value, which reads as every feature off.
func FetchSubnetFeatures(host string, fetchRootKey bool, subnetID principal.Principal, opts ...FetchOption) (SubnetFeatures, error) {
	a, err := newRegistryAgent(host, fetchRootKey, opts)
	if err != nil {
		return SubnetFeatures{}, fmt.Errorf("new registry agent: %w", err)
	}
	resp, err := a.GetSubnet(registry.GetSubnetRequest{SubnetId: &subnetID})
	if err != nil {
		return SubnetFeatures{}, fmt.Errorf("get_subnet: %w", err)
	}
	if resp.Err != nil {
		return SubnetFeatures{}, fmt.Errorf("registry rejected get_subnet: %s", *resp.Err)
	}
	if resp.Ok == nil {
		return SubnetFeatures{}, fmt.Errorf("get_subnet: empty response")
	}
	return SubnetFeatures{SevEnabled: resp.Ok.Features.SevEnabled}, nil
}

// FetchSubnetReplicaVersion returns a subnet's current GuestOS/replica version
// id from the registry. Read-only.
func FetchSubnetReplicaVersion(host string, fetchRootKey bool, subnetID principal.Principal, opts ...FetchOption) (string, error) {
	a, err := newRegistryAgent(host, fetchRootKey, opts)
	if err != nil {
		return "", fmt.Errorf("new registry agent: %w", err)
	}
	resp, err := a.GetSubnet(registry.GetSubnetRequest{SubnetId: &subnetID})
	if err != nil {
		return "", fmt.Errorf("get_subnet: %w", err)
	}
	if resp.Err != nil {
		return "", fmt.Errorf("registry rejected get_subnet: %s", *resp.Err)
	}
	if resp.Ok == nil {
		return "", fmt.Errorf("get_subnet: empty response")
	}
	return resp.Ok.ReplicaVersionId, nil
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
// the registry canister, no HTTP explorer. An empty slice means the provider is
// unknown to the registry.
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
