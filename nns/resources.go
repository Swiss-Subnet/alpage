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

// namedResource is implemented by every resource kind, so the eval context and
// label map can be built without knowing the concrete kind. gohcl does not
// flatten embedded structs, so each kind repeats name/id/label rather than
// sharing a base.
type namedResource interface {
	name() string
	id() string
	label() string
}

// Subnet is referenced from proposals and nodes as subnet.<name>.id.
type Subnet struct {
	Name  string `hcl:"name,label"`
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
	// SevEnabled is reconciled against the subnet record's features.sev_enabled.
	// Omitted means false: subnets are not SEV-enabled by default, so an
	// undeclared subnet that is enabled on-chain is drift. This is the
	// subnet-level flag; the per-node counterpart is NodeRes.SevEnabled, which
	// reconciles against the node record's chip_id.
	SevEnabled bool `hcl:"sev_enabled,optional"`
	// Type is the subnet type: application (the default when omitted),
	// verified_application, system, or cloud_engine. Omitted means application,
	// so a subnet whose type changes on-chain surfaces as drift rather than
	// being silently accepted.
	Type string `hcl:"type,optional"`
	// CostSchedule is the canister cycles cost schedule: normal (the default) or
	// free. Cloud engines must be free; the registry enforces it, so ValidateSubnet
	// rejects the combination before a proposal is cut.
	CostSchedule string `hcl:"cost_schedule,optional"`
	// Admins are the principals with admin rights on the subnet (subnet_admins
	// on the record). Allowed only on a cloud_engine or a rented subnet, which
	// the registry infers from application + free rather than a subnet type.
	// Declaring none asserts none: a subnet that gains an admin on-chain is
	// drift. Order is not meaningful.
	Admins []string `hcl:"admins,optional"`
}

// NodeProvider is referenced from node_operator as node_provider.<name>.id.
type NodeProvider struct {
	Name  string `hcl:"name,label"`
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
	// Decommissioned marks a provider removed on-chain. Its absence from the
	// registry is then the expected end state, not drift.
	Decommissioned bool `hcl:"decommissioned,optional"`
}

// DataCenter is referenced from node_operator as data_center.<name>.id.
type DataCenter struct {
	Name  string `hcl:"name,label"`
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
	// Region is the registry region string; reconcile compares it against the
	// on-chain dc region. Not exposed in the eval context (no .region refs).
	Region string `hcl:"region,optional"`
}

// NodeOperator is referenced from node as node_operator.<name>.id.
type NodeOperator struct {
	Name  string `hcl:"name,label"`
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
	// Provider is the id of its node provider.
	Provider string `hcl:"provider,optional"`
	// Dc is the id of its data center (data_center.<name>.id).
	Dc string `hcl:"dc,optional"`
	// Decommissioned marks an operator removed on-chain. Its absence from the
	// registry is then the expected end state, not drift.
	Decommissioned bool `hcl:"decommissioned,optional"`
}

// GuestosVersionRes names a GuestOS/replica version so many nodes can share one
// declaration, making a fleet-wide rollout a single edit. Referenced from node
// as guestos_version.<name>.id. Unlike the other kinds its id is a version hash
// rather than a principal, but it is spelled id so it resolves through the same
// <kind>.<name>.id form.
type GuestosVersionRes struct {
	Name  string `hcl:"name,label"`
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
	// Default marks the version every node is expected to run unless its own
	// block overrides it. At most one block may carry it.
	Default bool `hcl:"default,optional"`
}

// GuestosVersionNone opts one node out of the version check, which an omitted
// guestos_version no longer does once a default exists.
const GuestosVersionNone = "none"

// VersionSource says where a node's expected version came from.
type VersionSource string

const (
	VersionFromNode    VersionSource = "node"
	VersionFromDefault VersionSource = "default"
)

// VersionExpectation is the version one node is expected to run, and why.
type VersionExpectation struct {
	Version string
	Source  VersionSource
}

// NodeRes is a node resource, referenced from proposals as node.<name>.id.
// (Named NodeRes to avoid colliding with the proposal-payload Node in spec.go.)
type NodeRes struct {
	Name  string `hcl:"name,label"`
	ID    string `hcl:"id"`
	Label string `hcl:"label,optional"`
	// Subnet is the id of the subnet it belongs to (typically subnet.<name>.id).
	// Empty means unassigned; reconcile only diffs nodes that declare the subnet
	// under test.
	Subnet string `hcl:"subnet,optional"`
	// Operator is the id of its node operator (node_operator.<name>.id).
	Operator string `hcl:"operator,optional"`
	// Decommissioned marks a node that has been deregistered on-chain. Its block
	// is kept so historical proposal payloads keep resolving to the ids they were
	// submitted with; reconcile expects it to be absent from the registry, and
	// reports drift if it is still there.
	Decommissioned bool `hcl:"decommissioned,optional"`
	// GuestosVersion is the GuestOS/replica version this node is expected to be
	// running. Unlike every other reconciled field this is not a registry fact:
	// the registry stores a version per subnet, not per node, so it is read from
	// the node's own /api/v2/status impl_version. Omitted inherits the
	// guestos_version marked default, or means unchecked when there is none;
	// "none" opts out.
	GuestosVersion string `hcl:"guestos_version,optional"`
	// ChipID pins the node's AMD SEV-SNP CHIP_ID, base64 as the registry serves
	// it. Omitted asserts the node carries none, so gaining, losing, or
	// changing one is drift.
	ChipID string `hcl:"chip_id,optional"`
}

// DefaultVersion returns the guestos_version block marked default, if any.
func (r *Resources) DefaultVersion() (GuestosVersionRes, bool) {
	if r == nil {
		return GuestosVersionRes{}, false
	}
	for _, v := range r.Versions {
		if v.Default {
			return v, true
		}
	}
	return GuestosVersionRes{}, false
}

// ExpectedVersion resolves the version n is expected to run: its own
// guestos_version, else the default. ok is false when nothing is expected, so
// reconcile skips the node. The elected-version fetch, the short-circuit, and
// the per-node read must all agree on this set, or reconcile reports clean on
// nodes it never collected.
func (r *Resources) ExpectedVersion(n NodeRes) (VersionExpectation, bool) {
	switch {
	case n.Decommissioned, n.GuestosVersion == GuestosVersionNone:
		return VersionExpectation{}, false
	case n.GuestosVersion != "":
		return VersionExpectation{Version: n.GuestosVersion, Source: VersionFromNode}, true
	}
	if d, ok := r.DefaultVersion(); ok {
		return VersionExpectation{Version: d.ID, Source: VersionFromDefault}, true
	}
	return VersionExpectation{}, false
}

// VersionCheckedNodes are the nodes reconcile reads a running version from,
// each with what it is expected to be running.
func (r *Resources) VersionCheckedNodes() ([]NodeRes, []VersionExpectation) {
	if r == nil {
		return nil, nil
	}
	var nodes []NodeRes
	var want []VersionExpectation
	for _, n := range r.Nodes {
		if e, ok := r.ExpectedVersion(n); ok {
			nodes = append(nodes, n)
			want = append(want, e)
		}
	}
	return nodes, want
}

func (s Subnet) name() string        { return s.Name }
func (s Subnet) id() string          { return s.ID }
func (s Subnet) label() string       { return s.Label }
func (p NodeProvider) name() string  { return p.Name }
func (p NodeProvider) id() string    { return p.ID }
func (p NodeProvider) label() string { return p.Label }
func (d DataCenter) name() string    { return d.Name }
func (d DataCenter) id() string      { return d.ID }
func (d DataCenter) label() string   { return d.Label }
func (o NodeOperator) name() string  { return o.Name }
func (o NodeOperator) id() string    { return o.ID }
func (o NodeOperator) label() string { return o.Label }
func (n NodeRes) name() string       { return n.Name }
func (n NodeRes) id() string         { return n.ID }
func (n NodeRes) label() string      { return n.Label }

// Describe renders the version for humans: its release name where one is
// declared, the hash otherwise.
func (v GuestosVersionRes) Describe() string {
	if v.Label == "" {
		return v.ID
	}
	return fmt.Sprintf("%s (%s)", v.Label, v.ID)
}

func (v GuestosVersionRes) name() string  { return v.Name }
func (v GuestosVersionRes) id() string    { return v.ID }
func (v GuestosVersionRes) label() string { return v.Label }

// DefaultResourcesPath is where node/subnet resources live, relative to the
// module root.
const DefaultResourcesPath = "resources.hcl"

type resourcesFile struct {
	Nodes     []NodeRes           `hcl:"node,block"`
	Subnets   []Subnet            `hcl:"subnet,block"`
	Providers []NodeProvider      `hcl:"node_provider,block"`
	Operators []NodeOperator      `hcl:"node_operator,block"`
	DCs       []DataCenter        `hcl:"data_center,block"`
	Versions  []GuestosVersionRes `hcl:"guestos_version,block"`
}

// leafBlocks is the first-pass shape: it reads the reference-free blocks
// (subnets, providers, data centers, GuestOS versions), leaving the referencing
// blocks (operators ref providers/DCs, nodes ref operators/subnets/versions) as
// raw unevaluated bodies so their cross-references are not evaluated before the
// context resolving them exists.
type leafBlocks struct {
	Subnets   []Subnet            `hcl:"subnet,block"`
	Providers []NodeProvider      `hcl:"node_provider,block"`
	DCs       []DataCenter        `hcl:"data_center,block"`
	Versions  []GuestosVersionRes `hcl:"guestos_version,block"`
	Rest      hcl.Body            `hcl:",remain"`
}

// operatorBlocks is the second-pass shape: it reads node_operator blocks (which
// reference providers/DCs, resolved by then) while leaving node blocks raw, so
// a node's operator/subnet references are not evaluated before node_operator
// exists as a variable.
type operatorBlocks struct {
	Operators []NodeOperator `hcl:"node_operator,block"`
	Rest      hcl.Body       `hcl:",remain"`
}

// Resources holds all named resources plus a reverse label lookup by id, used
// to render a resource's own label when a referencing block omits one.
type Resources struct {
	Nodes     []NodeRes
	Subnets   []Subnet
	Providers []NodeProvider
	Operators []NodeOperator
	DCs       []DataCenter
	Versions  []GuestosVersionRes
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
	setVar(ctx, "guestos_version", leaf.Versions)

	var ops operatorBlocks
	if diags := gohcl.DecodeBody(body, ctx, &ops); diags.HasErrors() {
		return nil, fmt.Errorf("decode resources: %s", diags.Error())
	}
	setVar(ctx, "node_operator", ops.Operators)

	var rf resourcesFile
	if diags := gohcl.DecodeBody(body, ctx, &rf); diags.HasErrors() {
		return nil, fmt.Errorf("decode resources: %s", diags.Error())
	}
	// Collisions are checked before anything consumes the resources: a repeated
	// name would silently win in the eval context (last block parsed), and a
	// repeated id would silently win in the label map, in both cases resolving
	// references to a block the author did not mean.
	if err := checkUnique(body); err != nil {
		return nil, err
	}
	// Reject declarations the registry would refuse (a cloud engine off the free
	// schedule, admins on a subnet that may not have them) at load rather than
	// leaving them to surface as a failed proposal.
	for _, sn := range rf.Subnets {
		if err := ValidateSubnet(sn); err != nil {
			return nil, err
		}
	}
	if err := checkOneDefaultVersion(rf.Versions); err != nil {
		return nil, err
	}
	labels := map[string]string{}
	addLabels(labels, rf.Nodes)
	addLabels(labels, rf.Subnets)
	addLabels(labels, rf.Providers)
	addLabels(labels, rf.Operators)
	addLabels(labels, rf.DCs)
	return &Resources{
		Nodes: rf.Nodes, Subnets: rf.Subnets, Providers: rf.Providers,
		Operators: rf.Operators, DCs: rf.DCs, Versions: rf.Versions, labels: labels,
	}, nil
}

// checkOneDefaultVersion rejects a second default: with two, which one a node
// inherits would come down to block order.
func checkOneDefaultVersion(vs []GuestosVersionRes) error {
	first := ""
	for _, v := range vs {
		if !v.Default {
			continue
		}
		if first != "" {
			return fmt.Errorf("guestos_version %q and %q are both marked default; only one may be", first, v.Name)
		}
		first = v.Name
	}
	return nil
}

func setVar[T namedResource](ctx *hcl.EvalContext, name string, rs []T) {
	if v := objectsByName(rs); len(v) > 0 {
		ctx.Variables[name] = cty.ObjectVal(v)
	}
}

// resourceKinds are the block types carrying a name label, in the order they
// are reported.
var resourceKinds = []string{"subnet", "node", "node_provider", "node_operator", "data_center", "guestos_version"}

// checkUnique rejects a repeated name or id within a kind. It walks the merged
// body rather than the decoded structs so each collision reports the file and
// line of both blocks, which is what makes a collision spanning resources.hcl
// and resources/*.hcl actionable. Ids are compared within a kind only: distinct
// kinds may legitimately share a principal (a self-operated provider registers
// the same id as operator and provider).
func checkUnique(body hcl.Body) error {
	schema := &hcl.BodySchema{}
	for _, k := range resourceKinds {
		schema.Blocks = append(schema.Blocks, hcl.BlockHeaderSchema{Type: k, LabelNames: []string{"name"}})
	}
	content, _, diags := body.PartialContent(schema)
	if diags.HasErrors() {
		return fmt.Errorf("scan resources: %s", diags.Error())
	}
	names := map[string]hcl.Range{} // "kind.name" -> first declaration
	ids := map[string]hcl.Range{}   // "kind\x00id" -> first declaration
	for _, b := range content.Blocks {
		if len(b.Labels) == 0 {
			continue
		}
		key := b.Type + "." + b.Labels[0]
		if prev, ok := names[key]; ok {
			return fmt.Errorf("duplicate %s %q: declared at %s and %s", b.Type, b.Labels[0], prev, b.DefRange)
		}
		names[key] = b.DefRange
		// Versions are exempt: an alias block deliberately carries the same hash
		// as the catalogue entry it points at, and both resolve to that hash, so
		// there is no reference to get wrong.
		if b.Type == "guestos_version" {
			continue
		}
		id, err := blockID(b)
		if err != nil || id == "" {
			continue
		}
		idKey := b.Type + "\x00" + id
		if prev, ok := ids[idKey]; ok {
			return fmt.Errorf("duplicate %s id %q: declared at %s and %s", b.Type, id, prev, b.DefRange)
		}
		ids[idKey] = b.DefRange
	}
	return nil
}

// blockID reads a block's literal id attribute. Resource ids are string
// literals, so this evaluates with a nil context; a non-literal or absent id
// yields "" and is left to the main decode to accept or reject.
func blockID(b *hcl.Block) (string, error) {
	attrs, _, diags := b.Body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: "id"}},
	})
	if diags.HasErrors() {
		return "", fmt.Errorf("%s", diags.Error())
	}
	a, ok := attrs.Attributes["id"]
	if !ok {
		return "", nil
	}
	v, vdiags := a.Expr.Value(nil)
	if vdiags.HasErrors() || v.Type() != cty.String || v.IsNull() {
		return "", nil
	}
	return v.AsString(), nil
}

func addLabels[T namedResource](labels map[string]string, rs []T) {
	for _, r := range rs {
		labels[r.id()] = r.label()
	}
}

// EvalContext exposes the resource variables so proposal expressions like
// node.foo.id and subnet.bar.id resolve. Each resource is an object with id
// and label attributes.
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
	// Exported so a deploy_guestos proposal can name a version the same way
	// nodes do, rather than repeating the hash.
	setVar(ctx, "guestos_version", r.Versions)
	return ctx
}

func (r *Resources) LabelFor(id string) string {
	if r == nil {
		return ""
	}
	return r.labels[id]
}

func objectsByName[T namedResource](rs []T) map[string]cty.Value {
	out := make(map[string]cty.Value, len(rs))
	for _, r := range rs {
		out[r.name()] = cty.ObjectVal(map[string]cty.Value{
			"id":    cty.StringVal(r.id()),
			"label": cty.StringVal(r.label()),
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
