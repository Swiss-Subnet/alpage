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
	Since  string   // tag that introduced the block under this name
}

// SchemaBlocks is the full set of documented top-level HCL blocks. Nested blocks
// (a proposal's kind-specific body) are documented via their own entry with the
// nesting noted in Doc.
var SchemaBlocks = []SchemaBlock{
	{
		Name: "subnet", File: "resources.hcl", Labels: []string{"name"},
		Doc:   "A subnet, referenced from proposals and nodes as subnet.<name>.id.",
		Type:  Subnet{},
		Since: "v0.1.0",
	},
	{
		Name: "data_center", File: "resources.hcl", Labels: []string{"name"},
		Doc:   "A registry data center, referenced by node_operator as data_center.<name>.id.",
		Type:  DataCenter{},
		Since: "v0.1.0",
	},
	{
		Name: "node_provider", File: "resources.hcl", Labels: []string{"name"},
		Doc:   "A node provider, referenced by node_operator as node_provider.<name>.id.",
		Type:  NodeProvider{},
		Since: "v0.1.0",
	},
	{
		Name: "node_operator", File: "resources.hcl", Labels: []string{"name"},
		Doc:   "A node operator, referenced by node as node_operator.<name>.id.",
		Type:  NodeOperator{},
		Since: "v0.1.0",
	},
	{
		Name: "guestos_version", File: "resources.hcl", Labels: []string{"name"},
		Doc:   "A named GuestOS/replica version, referenced by node as guestos_version.<name>.id.",
		Type:  GuestosVersionRes{},
		Since: "v0.3.0",
	},
	{
		Name: "node", File: "resources.hcl", Labels: []string{"name"},
		Doc:   "A node, referenced from proposals as node.<name>.id.",
		Type:  NodeRes{},
		Since: "v0.1.0",
	},
	{
		Name: "provider", File: "proposals.hcl",
		Doc:   "Global submission settings; CLI flags override these.",
		Type:  Provider{},
		Since: "v0.1.0",
	},
	{
		Name: "proposal", File: "proposals.hcl", Labels: []string{"name"},
		Doc:   "One NNS proposal. Carries common metadata plus a nested block named after its kind.",
		Type:  Spec{},
		Since: "v0.1.0",
	},
	{
		Name: "membership", File: "proposals.hcl",
		Doc:   "Nested in a proposal of kind \"membership\": change_subnet_membership. Holds add/remove node blocks. Renamed from resize in v0.3.0; a config written for v0.2.0 or earlier must rename the block and its kind.",
		Type:  membershipBody{},
		Since: "v0.3.0",
	},
	{
		Name: "deploy_guestos", File: "proposals.hcl",
		Doc:   "Nested in a proposal of kind \"deploy_guestos\": deploy_guestos_to_all_subnet_nodes.",
		Type:  deployGuestosBody{},
		Since: "v0.1.0",
	},
	{
		Name: "add / remove", File: "proposals.hcl",
		Doc:   "Inside a membership block: a node to add to or remove from the subnet.",
		Type:  Node{},
		Since: "v0.1.0",
	},
}

// SchemaFieldDocs maps "<struct>.<hclname>" to a field description. schemadoc
// requires an entry for every non-label field on a documented struct; a missing
// entry (or an entry for a field that no longer exists) fails generation.
var SchemaFieldDocs = map[string]string{
	"Subnet.id":            "Principal of the subnet.",
	"Subnet.label":         "Human-readable name.",
	"Subnet.sev_enabled":   "Whether the subnet runs with SEV-SNP enabled, reconciled against the registry's features.sev_enabled. Omitted means false.",
	"Subnet.type":          "Subnet type: application, verified_application, system, or cloud_engine. Omitted means application.",
	"Subnet.cost_schedule": "Canister cycles cost schedule: normal or free. Omitted means free for a cloud_engine, which the registry requires, and normal otherwise.",
	"Subnet.admins":        "Principals with admin rights on the subnet (subnet_admins). Allowed only on a cloud_engine or a rented subnet (application on the free schedule), at most 10. Declaring none asserts none, so an admin added on-chain is drift. Order is not significant.",

	"DataCenter.id":     "Registry data center id.",
	"DataCenter.label":  "Human-readable name.",
	"DataCenter.region": "Registry region string (e.g. Europe,CH,Vaud).",

	"NodeProvider.id":    "Principal of the node provider.",
	"NodeProvider.label": "Human-readable name.",

	"NodeOperator.id":       "Principal of the node operator.",
	"NodeOperator.label":    "Human-readable name.",
	"NodeOperator.provider": "Id of its node provider (node_provider.<name>.id).",
	"NodeOperator.dc":       "Id of its data center (data_center.<name>.id).",

	"GuestosVersionRes.id":    "GuestOS/replica version hash. Spelled id, not hash, so it resolves through the same <kind>.<name>.id form as every other resource.",
	"GuestosVersionRes.label": "Human-readable name (e.g. the release name).",

	"NodeRes.id":              "Principal of the node.",
	"NodeRes.label":           "Human-readable name.",
	"NodeRes.subnet":          "Id of the subnet it belongs to (subnet.<name>.id). Empty means unassigned.",
	"NodeRes.operator":        "Id of its node operator (node_operator.<name>.id).",
	"NodeRes.decommissioned":  "Marks a node deregistered on-chain. Its block is kept so historical proposal payloads keep resolving to the ids they were submitted with; reconcile expects it to be absent from the registry.",
	"NodeRes.guestos_version": "GuestOS/replica version this node is expected to run. Not a registry fact: the registry stores one version per subnet, so reconcile reads the node's own /api/v2/status impl_version, which needs IPv6. If the node is unreachable it falls back to the public dashboard, marking the row \"via dashboard\" since that data may lag. Reconcile also checks the declared version against the NNS elected set and marks it \"NOT ELECTED\" if absent; when that source is unreadable the check is skipped rather than failing. Omitted means unchecked.",

	"Provider.host":           "Governance host URL. Defaults per command; overridden by --host.",
	"Provider.neuron":         "Proposer neuron id.",
	"Provider.fetch_root_key": "Whether to fetch the IC root key. Unset defaults to true for non-mainnet hosts.",

	"Spec.kind":    "Proposal kind; selects the nested block (membership, deploy_guestos).",
	"Spec.title":   "Proposal title shown on the NNS.",
	"Spec.summary": "Proposal summary (markdown).",
	"Spec.url":     "Reference URL (e.g. forum thread).",

	"membershipBody.subnet_id": "Subnet whose membership changes (subnet.<name>.id).",
	"membershipBody.add":       "A node to add to the subnet; repeatable. See the add / remove block.",
	"membershipBody.remove":    "A node to remove from the subnet; repeatable. See the add / remove block.",

	"deployGuestosBody.subnet_id":          "Subnet whose nodes to upgrade (subnet.<name>.id).",
	"deployGuestosBody.replica_version_id": "Replica version to deploy to every node in the subnet. Must be elected by the NNS: preflight checks the registry for a replica_version_<id> record (read via the registry explorer) and refuses an unelected version, since the NNS would reject the proposal. --force submits anyway, and also lets apply proceed when that lookup fails (downgraded to a warning); plan always degrades that way. Preflight additionally resolves the version's release name and election proposal from the public dashboard, which is display-only and degrades to a note when unavailable.",

	"Node.id":    "Node id (node.<name>.id).",
	"Node.label": "Optional human-readable name.",
}

// SchemaUnreleased is the version Since values carry before that version is
// tagged; the docs list it as unreleased rather than linking a tag that does
// not exist yet. Bump it when cutting a release that changes the schema.
const SchemaUnreleased = "v0.3.0"

// SchemaFieldSince maps "<struct>.<hclname>" to the release tag that introduced
// the field, so the generated docs say which version a reader's binary needs.
// A field renamed or moved to a renamed block counts as new at the rename.
// Kept in lockstep with SchemaFieldDocs by TestSchemaFieldSinceCoverage.
var SchemaFieldSince = map[string]string{
	"Subnet.id":            "v0.1.0",
	"Subnet.label":         "v0.1.0",
	"Subnet.sev_enabled":   "v0.1.1",
	"Subnet.type":          "v0.2.0",
	"Subnet.cost_schedule": "v0.2.0",
	"Subnet.admins":        "v0.2.0",

	"DataCenter.id":     "v0.1.0",
	"DataCenter.label":  "v0.1.0",
	"DataCenter.region": "v0.1.0",

	"NodeProvider.id":    "v0.1.0",
	"NodeProvider.label": "v0.1.0",

	"NodeOperator.id":       "v0.1.0",
	"NodeOperator.label":    "v0.1.0",
	"NodeOperator.provider": "v0.1.0",
	"NodeOperator.dc":       "v0.1.0",

	"GuestosVersionRes.id":    "v0.3.0",
	"GuestosVersionRes.label": "v0.3.0",

	"NodeRes.id":              "v0.1.0",
	"NodeRes.label":           "v0.1.0",
	"NodeRes.subnet":          "v0.1.0",
	"NodeRes.operator":        "v0.1.0",
	"NodeRes.decommissioned":  "v0.1.1",
	"NodeRes.guestos_version": "v0.2.0",

	"Provider.host":           "v0.1.0",
	"Provider.neuron":         "v0.1.0",
	"Provider.fetch_root_key": "v0.1.0",

	"Spec.kind":    "v0.1.0",
	"Spec.title":   "v0.1.0",
	"Spec.summary": "v0.1.0",
	"Spec.url":     "v0.1.0",

	// The block was resize until v0.3.0; its fields date from the rename.
	"membershipBody.subnet_id": "v0.3.0",
	"membershipBody.add":       "v0.3.0",
	"membershipBody.remove":    "v0.3.0",

	"deployGuestosBody.subnet_id":          "v0.1.0",
	"deployGuestosBody.replica_version_id": "v0.1.0",

	"Node.id":    "v0.1.0",
	"Node.label": "v0.1.0",
}
