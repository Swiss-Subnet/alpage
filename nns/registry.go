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

// FetchSubnetMembership returns a subnet's node ids as sorted textual
// principals. Read-only query: no identity, host defaults to mainnet.
func FetchSubnetMembership(host string, fetchRootKey bool, subnetID principal.Principal) ([]string, error) {
	if host == "" {
		host = MainnetHost
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse host %q: %w", host, err)
	}
	a, err := registry.NewRegistryAgent(RegistryID, agent.Config{
		ClientConfig: []agent.ClientOption{agent.WithHostURL(u)},
		FetchRootKey: fetchRootKey,
	})
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
