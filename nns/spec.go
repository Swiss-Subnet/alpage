package nns

import (
	"fmt"
	"strings"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

type Node struct {
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
}

type membershipBody struct {
	SubnetID string `hcl:"subnet_id"`
	Add      []Node `hcl:"add,block"`
	Remove   []Node `hcl:"remove,block"`
}

type deployGuestosBody struct {
	SubnetID         string `hcl:"subnet_id"`
	ReplicaVersionID string `hcl:"replica_version_id"`
}

type removeNodesBody struct {
	Nodes []Node `hcl:"node,block"`
}

type removeNodeOperatorsBody struct {
	Operators []string `hcl:"operators"`
}

type removeNodeProviderBody struct {
	NodeProviderID string `hcl:"node_provider_id"`
}

type addDataCentersBody struct {
	DataCenters []dataCenterBody `hcl:"dc,block"`
}

type dataCenterBody struct {
	ID     string   `hcl:"id"`
	Region string   `hcl:"region"`
	Owner  string   `hcl:"owner"`
	GPS    *gpsBody `hcl:"gps,block"`
}

type gpsBody struct {
	Latitude  float64 `hcl:"latitude"`
	Longitude float64 `hcl:"longitude"`
}

type addNodeProviderBody struct {
	NodeProviderID string `hcl:"node_provider_id"`
	RewardAccount  string `hcl:"reward_account,optional"`
}

type addNodeOperatorBody struct {
	NodeOperatorID     string            `hcl:"node_operator_id"`
	NodeProviderID     string            `hcl:"node_provider_id"`
	DcID               string            `hcl:"dc_id"`
	NodeAllowance      uint64            `hcl:"node_allowance,optional"`
	IPv6               string            `hcl:"ipv6,optional"`
	MaxRewardableNodes map[string]uint32 `hcl:"max_rewardable_nodes,optional"`
}

// updateSubnetBody mirrors the settings update_subnet can change. Every
// top-level field is optional and a pointer, because the registry skips an unset
// field rather than resetting it: nil has to mean "do not touch", which a bare
// bool or uint64 cannot express. The nested blocks are the opposite -- sending
// one replaces the subnet's whole record, so all of their fields are required.
type updateSubnetBody struct {
	SubnetID string `hcl:"subnet_id"`

	IsHalted             *bool     `hcl:"is_halted,optional"`
	HaltAtCupHeight      *bool     `hcl:"halt_at_cup_height,optional"`
	MaxNumberOfCanisters *uint64   `hcl:"max_number_of_canisters,optional"`
	SSHReadonlyAccess    *[]string `hcl:"ssh_readonly_access,optional"`
	SSHBackupAccess      *[]string `hcl:"ssh_backup_access,optional"`
	SubnetAdmins         *[]string `hcl:"subnet_admins,optional"`

	Features       *subnetFeaturesBody `hcl:"features,block"`
	ResourceLimits *resourceLimitsBody `hcl:"resource_limits,block"`

	UnitDelayMillis            *uint64 `hcl:"unit_delay_millis,optional"`
	InitialNotaryDelayMillis   *uint64 `hcl:"initial_notary_delay_millis,optional"`
	DkgIntervalLength          *uint64 `hcl:"dkg_interval_length,optional"`
	DkgDealingsPerBlock        *uint64 `hcl:"dkg_dealings_per_block,optional"`
	MaxIngressBytesPerMessage  *uint64 `hcl:"max_ingress_bytes_per_message,optional"`
	MaxIngressBytesPerBlock    *uint64 `hcl:"max_ingress_bytes_per_block,optional"`
	MaxIngressMessagesPerBlock *uint64 `hcl:"max_ingress_messages_per_block,optional"`
	MaxBlockPayloadSize        *uint64 `hcl:"max_block_payload_size,optional"`
}

type subnetFeaturesBody struct {
	CanisterSandboxing bool `hcl:"canister_sandboxing"`
	HTTPRequests       bool `hcl:"http_requests"`
	SevEnabled         bool `hcl:"sev_enabled"`
}

type resourceLimitsBody struct {
	MaximumStateSize  uint64 `hcl:"maximum_state_size"`
	MaximumStateDelta uint64 `hcl:"maximum_state_delta"`
}

// Spec is one `proposal "<name>" { ... }` block: common metadata plus a
// kind-specific nested block. Single source of truth for the payload; dry-run,
// submit, and state all derive from it.
type Spec struct {
	Name    string   `hcl:"name,label"`
	Kind    string   `hcl:"kind"`
	Title   string   `hcl:"title"`
	Summary string   `hcl:"summary,optional"`
	URL     string   `hcl:"url,optional"`
	Rest    hcl.Body `hcl:",remain"`

	// evalCtx resolves node.<name>.id / subnet.<name>.id references in the
	// nested block; set by LoadConfig from resources.hcl.
	evalCtx *hcl.EvalContext
}

func (s *Spec) Meta() Meta { return Meta{Title: s.Title, Summary: s.Summary, URL: s.URL} }

type configFile struct {
	Provider  *Provider `hcl:"provider,block"`
	Proposals []Spec    `hcl:"proposal,block"`
}

// Provider is the global submission config. FetchRootKey is a pointer so
// "unset" (derive from host) is distinguishable from an explicit false.
type Provider struct {
	Host         string `hcl:"host,optional"`
	Neuron       uint64 `hcl:"neuron,optional"`
	FetchRootKey *bool  `hcl:"fetch_root_key,optional"`
}

// ShouldFetchRootKey defaults to true for non-mainnet hosts (local/test
// networks need the root key), unless the provider sets it explicitly.
func (p Provider) ShouldFetchRootKey(host string) bool {
	if p.FetchRootKey != nil {
		return *p.FetchRootKey
	}
	return host != MainnetHost
}

// ResolveTarget applies host precedence (flag > provider > mainnet) and derives
// fetch_root_key from the result. Every command that talks to a host resolves
// both here: deriving the flag from the host alone drops an explicit
// provider.fetch_root_key = false.
func (p Provider) ResolveTarget(flagHost string) (host string, fetchRootKey bool) {
	switch {
	case flagHost != "":
		host = flagHost
	case p.Host != "":
		host = p.Host
	default:
		host = MainnetHost
	}
	return host, p.ShouldFetchRootKey(host)
}

type Config struct {
	Provider  Provider
	Resources *Resources
	Proposals []Spec
}

const DefaultConfigPath = "proposals.hcl"

// proposalURLDomain is governance's entire allowlist for the proposal url. It
// is matched exactly against everything between the scheme and the first slash,
// so a subdomain or an explicit port is rejected.
const proposalURLDomain = "forum.dfinity.org"

// validateProposalURL mirrors validate_proposal_url in governance, which only
// reports a rejected url after the payload has already been built and submitted.
// An empty url is legal there, so it is legal here.
func validateProposalURL(url string) error {
	if url == "" {
		return nil
	}
	if n := len(url); n < 10 || n > 2048 {
		return fmt.Errorf("url is %d characters, outside governance's range of 10 to 2048", n)
	}
	rest, ok := strings.CutPrefix(url, "https://")
	if !ok {
		return fmt.Errorf("url %q must begin with https://", url)
	}
	if rest == "" {
		return fmt.Errorf("url %q has nothing after the scheme", url)
	}
	if strings.Contains(rest, "://") {
		return fmt.Errorf("url %q contains more than one scheme", url)
	}
	if strings.Contains(url, "@") {
		return fmt.Errorf("url %q must not carry authentication information", url)
	}
	if domain, _, _ := strings.Cut(rest, "/"); domain != proposalURLDomain {
		return fmt.Errorf("url %q is on %q, and governance accepts only %s", url, domain, proposalURLDomain)
	}
	return nil
}

// LoadConfig parses proposals.hcl, resolving node/subnet references against the
// resources.hcl sitting alongside it.
func LoadConfig(path string) (*Config, error) {
	resources, err := loadResources(resourcesPathFor(path))
	if err != nil {
		return nil, err
	}
	evalCtx := resources.EvalContext()

	p := hclparse.NewParser()
	f, diags := p.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse %s: %s", path, diags.Error())
	}
	var cfg configFile
	if diags := gohcl.DecodeBody(f.Body, evalCtx, &cfg); diags.HasErrors() {
		return nil, fmt.Errorf("decode %s: %s", path, diags.Error())
	}
	// A proposal name keys into lifecycle state and is how LoadSpec selects one,
	// so a duplicate would silently hide the second block behind the first.
	seen := map[string]bool{}
	for i := range cfg.Proposals {
		name := cfg.Proposals[i].Name
		if seen[name] {
			return nil, fmt.Errorf("%s: duplicate proposal %q: declared more than once", path, name)
		}
		seen[name] = true
		cfg.Proposals[i].evalCtx = evalCtx
		if err := validateProposalURL(cfg.Proposals[i].URL); err != nil {
			return nil, fmt.Errorf("%s: proposal %q: %w", path, name, err)
		}
		if _, err := cfg.Proposals[i].Action(); err != nil {
			return nil, err
		}
	}
	out := &Config{Resources: resources, Proposals: cfg.Proposals}
	if cfg.Provider != nil {
		out.Provider = *cfg.Provider
	}
	return out, nil
}

func LoadSpec(path, name string) (*Spec, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	for i := range cfg.Proposals {
		if cfg.Proposals[i].Name == name {
			return &cfg.Proposals[i], nil
		}
	}
	return nil, fmt.Errorf("proposal %q not found in %s", name, path)
}

// Action decodes the kind-specific nested block into the concrete Action.
// Single dispatch point: a new proposal type is one entry in the kinds table,
// which is also where its docs and its payload renderer come from.
func (s *Spec) Action() (Action, error) {
	k, ok := kindByName(s.Kind)
	if !ok {
		return nil, fmt.Errorf("proposal %q: unsupported kind %q (known kinds: %s)", s.Name, s.Kind, kindList())
	}
	return k.Decode(s)
}

// decodeRest pulls the single nested block named after the spec's kind out of
// the remaining body and decodes it into v. gohcl cannot target a
// dynamically-named block via struct tags, so we select it by name here.
func (s *Spec) decodeRest(v any) error {
	content, _, diags := s.Rest.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: s.Kind}},
	})
	if diags.HasErrors() {
		return fmt.Errorf("proposal %q: %s", s.Name, diags.Error())
	}
	if len(content.Blocks) != 1 {
		return fmt.Errorf("proposal %q: expected exactly one %q block, got %d", s.Name, s.Kind, len(content.Blocks))
	}
	if diags := gohcl.DecodeBody(content.Blocks[0].Body, s.evalCtx, v); diags.HasErrors() {
		return fmt.Errorf("proposal %q: %s", s.Name, diags.Error())
	}
	return nil
}

func decodeNodes(ns []Node) ([]principal.Principal, error) {
	out := make([]principal.Principal, len(ns))
	for i, n := range ns {
		p, err := principal.Decode(n.ID)
		if err != nil {
			return nil, fmt.Errorf("node id %q: %w", n.ID, err)
		}
		out[i] = p
	}
	return out, nil
}

// Proposal returns the membership proposal for a membership spec (used by the reproduce
// command and e2e tests), erroring if the spec is not a membership change.
func (s *Spec) Proposal() (MembershipProposal, error) {
	a, err := s.Action()
	if err != nil {
		return MembershipProposal{}, err
	}
	r, ok := a.(MembershipProposal)
	if !ok {
		return MembershipProposal{}, fmt.Errorf("proposal %q is kind %q, not membership", s.Name, s.Kind)
	}
	return r, nil
}

// PayloadSHA256 hashes the exact candid payload blob, not the config
// formatting, so state detects whether a resubmission carries the same bytes.
func (s *Spec) PayloadSHA256() (string, error) {
	a, err := s.Action()
	if err != nil {
		return "", err
	}
	return payloadSHA256(a)
}
