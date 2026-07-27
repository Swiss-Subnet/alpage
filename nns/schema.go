package nns

// The HCL config schema is defined by the `hcl:"..."` struct tags on the types
// below; docs/config.md is generated from them by cmd/schemadoc (go generate
// ./nns). This file supplies the human prose the tags cannot: a description per
// block and per field. schemadoc fails if a block/field is missing here or if a
// documented field no longer exists, so the docs cannot silently drift.

//go:generate go run ../cmd/schemadoc -out ../docs/config.md

// SchemaBlock ties an HCL block name to the struct that decodes it, the file it
// appears in, and a one-line description.
type SchemaBlock struct {
	Name   string   // HCL block type, e.g. "node_operator"
	File   string   // config file it belongs to, for grouping in the docs
	Doc    string   // one-line description of the block
	Type   any      // zero value of the decoding struct; reflected for fields
	Labels []string // block label names, e.g. ["name"] for node "foo" {}
}

// SchemaBlocks is the full set of documented top-level HCL blocks. Nested blocks
// (a proposal's kind-specific body) are documented via their own entry with the
// nesting noted in Doc.
var SchemaBlocks = []SchemaBlock{
	{
		Name: "subnet", File: "resources.hcl", Labels: []string{"name"},
		Doc:  "A subnet, referenced from proposals and nodes as subnet.<name>.id.",
		Type: Subnet{},
	},
	{
		Name: "data_center", File: "resources.hcl", Labels: []string{"name"},
		Doc:  "A registry data center, referenced by node_operator as data_center.<name>.id.",
		Type: DataCenter{},
	},
	{
		Name: "node_provider", File: "resources.hcl", Labels: []string{"name"},
		Doc:  "A node provider, referenced by node_operator as node_provider.<name>.id.",
		Type: NodeProvider{},
	},
	{
		Name: "node_operator", File: "resources.hcl", Labels: []string{"name"},
		Doc:  "A node operator, referenced by node as node_operator.<name>.id.",
		Type: NodeOperator{},
	},
	{
		Name: "node", File: "resources.hcl", Labels: []string{"name"},
		Doc:  "A node, referenced from proposals as node.<name>.id.",
		Type: NodeRes{},
	},
	{
		Name: "provider", File: "proposals.hcl",
		Doc:  "Global submission settings; CLI flags override these.",
		Type: Provider{},
	},
	{
		Name: "proposal", File: "proposals.hcl", Labels: []string{"name"},
		Doc:  "One NNS proposal. Carries common metadata plus a nested block named after its kind.",
		Type: Spec{},
	},
	{
		Name: "resize", File: "proposals.hcl",
		Doc:  "Nested in a proposal of kind \"resize\": change_subnet_membership. Holds add/remove node blocks.",
		Type: resizeBody{},
	},
	{
		Name: "deploy_guestos", File: "proposals.hcl",
		Doc:  "Nested in a proposal of kind \"deploy_guestos\": deploy_guestos_to_all_subnet_nodes.",
		Type: deployGuestosBody{},
	},
	{
		Name: "add / remove", File: "proposals.hcl",
		Doc:  "Inside a resize block: a node to add to or remove from the subnet.",
		Type: Node{},
	},
}

// SchemaFieldDocs maps "<struct>.<hclname>" to a field description. schemadoc
// requires an entry for every non-label field on a documented struct; a missing
// entry (or an entry for a field that no longer exists) fails generation.
var SchemaFieldDocs = map[string]string{
	"Subnet.id":          "Principal of the subnet.",
	"Subnet.label":       "Human-readable name.",
	"Subnet.sev_enabled": "Whether the subnet runs with SEV-SNP enabled, reconciled against the registry's features.sev_enabled. Omitted means false.",

	"DataCenter.id":     "Registry data center id.",
	"DataCenter.label":  "Human-readable name.",
	"DataCenter.region": "Registry region string (e.g. Europe,CH,Vaud).",

	"NodeProvider.id":    "Principal of the node provider.",
	"NodeProvider.label": "Human-readable name.",

	"NodeOperator.id":       "Principal of the node operator.",
	"NodeOperator.label":    "Human-readable name.",
	"NodeOperator.provider": "Id of its node provider (node_provider.<name>.id).",
	"NodeOperator.dc":       "Id of its data center (data_center.<name>.id).",

	"NodeRes.id":             "Principal of the node.",
	"NodeRes.label":          "Human-readable name.",
	"NodeRes.subnet":         "Id of the subnet it belongs to (subnet.<name>.id). Empty means unassigned.",
	"NodeRes.operator":       "Id of its node operator (node_operator.<name>.id).",
	"NodeRes.decommissioned": "Marks a node deregistered on-chain. Its block is kept so historical proposal payloads keep resolving to the ids they were submitted with; reconcile expects it to be absent from the registry.",

	"Provider.host":           "Governance host URL. Defaults per command; overridden by --host.",
	"Provider.neuron":         "Proposer neuron id.",
	"Provider.fetch_root_key": "Whether to fetch the IC root key. Unset defaults to true for non-mainnet hosts.",

	"Spec.kind":    "Proposal kind; selects the nested block (resize, deploy_guestos).",
	"Spec.title":   "Proposal title shown on the NNS.",
	"Spec.summary": "Proposal summary (markdown).",
	"Spec.url":     "Reference URL (e.g. forum thread).",

	"resizeBody.subnet_id": "Subnet to resize (subnet.<name>.id).",
	"resizeBody.add":       "A node to add to the subnet; repeatable. See the add / remove block.",
	"resizeBody.remove":    "A node to remove from the subnet; repeatable. See the add / remove block.",

	"deployGuestosBody.subnet_id":          "Subnet whose nodes to upgrade (subnet.<name>.id).",
	"deployGuestosBody.replica_version_id": "Replica version to deploy to every node in the subnet.",

	"Node.id":    "Node id (node.<name>.id).",
	"Node.label": "Optional human-readable name.",
}
