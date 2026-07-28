package nns

import (
	"fmt"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

type Node struct {
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
}

type resizeBody struct {
	SubnetID string `hcl:"subnet_id"`
	Add      []Node `hcl:"add,block"`
	Remove   []Node `hcl:"remove,block"`
}

type deployGuestosBody struct {
	SubnetID         string `hcl:"subnet_id"`
	ReplicaVersionID string `hcl:"replica_version_id"`
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

type Config struct {
	Provider  Provider
	Resources *Resources
	Proposals []Spec
}

const DefaultConfigPath = "proposals.hcl"

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
// Single dispatch point: a new proposal type is a case here plus a body struct.
func (s *Spec) Action() (Action, error) {
	switch s.Kind {
	case "resize":
		var body resizeBody
		if err := s.decodeRest(&body); err != nil {
			return nil, err
		}
		subnet, err := principal.Decode(body.SubnetID)
		if err != nil {
			return nil, fmt.Errorf("proposal %q: subnet_id %q: %w", s.Name, body.SubnetID, err)
		}
		add, err := decodeNodes(body.Add)
		if err != nil {
			return nil, fmt.Errorf("proposal %q: %w", s.Name, err)
		}
		remove, err := decodeNodes(body.Remove)
		if err != nil {
			return nil, fmt.Errorf("proposal %q: %w", s.Name, err)
		}
		return ResizeProposal{
			Meta:     s.Meta(),
			SubnetID: subnet, NodeIDsAdd: add, NodeIDsRemove: remove,
		}, nil
	case "deploy_guestos":
		var body deployGuestosBody
		if err := s.decodeRest(&body); err != nil {
			return nil, err
		}
		subnet, err := principal.Decode(body.SubnetID)
		if err != nil {
			return nil, fmt.Errorf("proposal %q: subnet_id %q: %w", s.Name, body.SubnetID, err)
		}
		if body.ReplicaVersionID == "" {
			return nil, fmt.Errorf("proposal %q: replica_version_id is required", s.Name)
		}
		return DeployGuestosAction{Meta: s.Meta(), SubnetID: subnet, ReplicaVersionID: body.ReplicaVersionID}, nil
	default:
		return nil, fmt.Errorf("proposal %q: unsupported kind %q", s.Name, s.Kind)
	}
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

// Proposal returns the resize proposal for a resize spec (used by the reproduce
// command and e2e tests), erroring if the spec is not a resize.
func (s *Spec) Proposal() (ResizeProposal, error) {
	a, err := s.Action()
	if err != nil {
		return ResizeProposal{}, err
	}
	r, ok := a.(ResizeProposal)
	if !ok {
		return ResizeProposal{}, fmt.Errorf("proposal %q is kind %q, not resize", s.Name, s.Kind)
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
