package nns

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// Resource is a named registry entity referenced from other blocks via an HCL
// expression like node.<name>.id / subnet.<name>.id / node_provider.<name>.id.
// The struct is shared across kinds; only the fields relevant to a kind are set.
type Resource struct {
	Name  string `hcl:"name,label"`
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
	// Subnet, on a node, is the id of the subnet it belongs to (typically
	// subnet.<name>.id). Empty means unassigned; reconcile only diffs nodes
	// that declare the subnet under test.
	Subnet string `hcl:"subnet,optional"`
	// Operator, on a node, is the id of its node operator (node_operator.<n>.id).
	Operator string `hcl:"operator,optional"`
	// Provider, on a node_operator, is the id of its node provider.
	Provider string `hcl:"provider,optional"`
	// Dc, on a node_operator, is the id of its data center (data_center.<n>.id).
	Dc string `hcl:"dc,optional"`
	// Region, on a data_center, is its registry region string.
	Region string `hcl:"region,optional"`
}

// DefaultResourcesPath is where node/subnet resources live, relative to the
// module root.
const DefaultResourcesPath = "resources.hcl"

type resourcesFile struct {
	Nodes     []Resource `hcl:"node,block"`
	Subnets   []Resource `hcl:"subnet,block"`
	Providers []Resource `hcl:"node_provider,block"`
	Operators []Resource `hcl:"node_operator,block"`
	DCs       []Resource `hcl:"data_center,block"`
}

// leafBlocks is the first-pass shape: it reads the reference-free blocks
// (subnets, providers, data centers), leaving the referencing blocks (operators
// ref providers/DCs, nodes ref operators/subnets) as raw unevaluated bodies so
// their cross-references are not evaluated before the context resolving them
// exists.
type leafBlocks struct {
	Subnets   []Resource `hcl:"subnet,block"`
	Providers []Resource `hcl:"node_provider,block"`
	DCs       []Resource `hcl:"data_center,block"`
	Rest      hcl.Body   `hcl:",remain"`
}

// operatorBlocks is the second-pass shape: it reads node_operator blocks (which
// reference providers/DCs, resolved by then) while leaving node blocks raw, so
// a node's operator/subnet references are not evaluated before node_operator
// exists as a variable.
type operatorBlocks struct {
	Operators []Resource `hcl:"node_operator,block"`
	Rest      hcl.Body   `hcl:",remain"`
}

// Resources holds all named resources plus a reverse label lookup by id, used
// to render a resource's own label when a referencing block omits one.
type Resources struct {
	Nodes     []Resource
	Subnets   []Resource
	Providers []Resource
	Operators []Resource
	DCs       []Resource
	labels    map[string]string // id -> label
}

// loadResources parses node and subnet resources. It reads path (resources.hcl)
// if present, plus any resources/*.hcl beside it, merging all files into one
// body so cross-file references (a node in one file referencing an operator in
// another) resolve. No files at all yields an empty set, so a config using only
// inline ids needs no resources.
func loadResources(path string) (*Resources, error) {
	files, err := resourceHCLPaths(path)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return &Resources{labels: map[string]string{}}, nil
	}
	p := hclparse.NewParser()
	parsed := make([]*hcl.File, 0, len(files))
	for _, fp := range files {
		f, diags := p.ParseHCLFile(fp)
		if diags.HasErrors() {
			return nil, fmt.Errorf("parse %s: %s", fp, diags.Error())
		}
		parsed = append(parsed, f)
	}
	body := hcl.MergeFiles(parsed)
	// Resolve references in dependency tiers. Tier 1 (subnets, providers, DCs)
	// reference nothing. Operators reference providers and DCs. Nodes reference
	// operators and subnets. Each decode runs against a context carrying the
	// tiers already resolved; a block's own body stays unevaluated until its
	// referents exist as variables.
	var leaf leafBlocks
	if diags := gohcl.DecodeBody(body, nil, &leaf); diags.HasErrors() {
		return nil, fmt.Errorf("decode resources: %s", diags.Error())
	}
	ctx := &hcl.EvalContext{Variables: map[string]cty.Value{}}
	setVar(ctx, "subnet", leaf.Subnets)
	setVar(ctx, "node_provider", leaf.Providers)
	setVar(ctx, "data_center", leaf.DCs)

	var ops operatorBlocks
	if diags := gohcl.DecodeBody(body, ctx, &ops); diags.HasErrors() {
		return nil, fmt.Errorf("decode resources: %s", diags.Error())
	}
	setVar(ctx, "node_operator", ops.Operators)

	var rf resourcesFile
	if diags := gohcl.DecodeBody(body, ctx, &rf); diags.HasErrors() {
		return nil, fmt.Errorf("decode resources: %s", diags.Error())
	}
	labels := map[string]string{}
	for _, group := range [][]Resource{rf.Nodes, rf.Subnets, rf.Providers, rf.Operators, rf.DCs} {
		for _, r := range group {
			labels[r.ID] = r.Label
		}
	}
	return &Resources{
		Nodes: rf.Nodes, Subnets: rf.Subnets, Providers: rf.Providers,
		Operators: rf.Operators, DCs: rf.DCs, labels: labels,
	}, nil
}

func setVar(ctx *hcl.EvalContext, name string, rs []Resource) {
	if v := objectsByName(rs); len(v) > 0 {
		ctx.Variables[name] = cty.ObjectVal(v)
	}
}

// EvalContext exposes the resource variables so proposal expressions like
// node.foo.id and subnet.bar.id resolve. Each resource is an object with id,
// label and region attributes.
func (r *Resources) EvalContext() *hcl.EvalContext {
	ctx := &hcl.EvalContext{Variables: map[string]cty.Value{}}
	if r == nil {
		return ctx
	}
	setVar(ctx, "node", r.Nodes)
	setVar(ctx, "subnet", r.Subnets)
	setVar(ctx, "node_provider", r.Providers)
	setVar(ctx, "node_operator", r.Operators)
	setVar(ctx, "data_center", r.DCs)
	return ctx
}

func (r *Resources) LabelFor(id string) string {
	if r == nil {
		return ""
	}
	return r.labels[id]
}

func objectsByName(rs []Resource) map[string]cty.Value {
	out := make(map[string]cty.Value, len(rs))
	for _, r := range rs {
		out[r.Name] = cty.ObjectVal(map[string]cty.Value{
			"id":     cty.StringVal(r.ID),
			"label":  cty.StringVal(r.Label),
			"region": cty.StringVal(r.Region),
		})
	}
	return out
}

func resourcesPathFor(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), DefaultResourcesPath)
}

// ResourcesDir is the directory beside resources.hcl holding split resource
// files (one per operator, plus shared subnets/providers/data centers). Every
// *.hcl in it is merged with resources.hcl.
const ResourcesDir = "resources"

// resourceHCLPaths returns the resource files to merge: resources.hcl (if it
// exists) followed by any resources/*.hcl beside it, sorted for stable merge
// order. A path pointing at neither yields an empty list.
func resourceHCLPaths(path string) ([]string, error) {
	var out []string
	if _, err := os.Stat(path); err == nil {
		out = append(out, path)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	dir := filepath.Join(filepath.Dir(path), ResourcesDir)
	matches, err := filepath.Glob(filepath.Join(dir, "*.hcl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return append(out, matches...), nil
}
