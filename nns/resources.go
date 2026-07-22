package nns

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// Resource is a named entity (a node or a subnet) referenced from proposals via
// an HCL expression: node.<name>.id or subnet.<name>.id.
type Resource struct {
	Name  string `hcl:"name,label"`
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
}

// DefaultResourcesPath is where node/subnet resources live, relative to the
// module root.
const DefaultResourcesPath = "resources.hcl"

type resourcesFile struct {
	Nodes   []Resource `hcl:"node,block"`
	Subnets []Resource `hcl:"subnet,block"`
}

// Resources holds all named resources plus a reverse label lookup by id, used
// to render a resource's own label when a referencing block omits one.
type Resources struct {
	Nodes   []Resource
	Subnets []Resource
	labels  map[string]string // id -> label
}

// loadResources parses node and subnet resources from path. A missing file
// yields an empty set, so a config using only inline ids needs no resources.hcl.
func loadResources(path string) (*Resources, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Resources{labels: map[string]string{}}, nil
	}
	p := hclparse.NewParser()
	f, diags := p.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse %s: %s", path, diags.Error())
	}
	var rf resourcesFile
	if diags := gohcl.DecodeBody(f.Body, nil, &rf); diags.HasErrors() {
		return nil, fmt.Errorf("decode %s: %s", path, diags.Error())
	}
	labels := map[string]string{}
	for _, r := range rf.Nodes {
		labels[r.ID] = r.Label
	}
	for _, r := range rf.Subnets {
		labels[r.ID] = r.Label
	}
	return &Resources{Nodes: rf.Nodes, Subnets: rf.Subnets, labels: labels}, nil
}

// EvalContext exposes the `node` and `subnet` variables so proposal expressions
// like node.foo.id and subnet.bar.id resolve. Each resource is an object with
// id and label attributes.
func (r *Resources) EvalContext() *hcl.EvalContext {
	ctx := &hcl.EvalContext{Variables: map[string]cty.Value{}}
	if r == nil {
		return ctx
	}
	if v := objectsByName(r.Nodes); len(v) > 0 {
		ctx.Variables["node"] = cty.ObjectVal(v)
	}
	if v := objectsByName(r.Subnets); len(v) > 0 {
		ctx.Variables["subnet"] = cty.ObjectVal(v)
	}
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
			"id":    cty.StringVal(r.ID),
			"label": cty.StringVal(r.Label),
		})
	}
	return out
}

func resourcesPathFor(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), DefaultResourcesPath)
}
