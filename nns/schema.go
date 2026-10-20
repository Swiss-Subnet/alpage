package nns

import "slices"

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

// SchemaBlocks is the full set of documented HCL blocks, in the order the docs
// present them: the resource and top-level blocks, then one per proposal kind
// from the kinds table, then the blocks nested inside those.
var SchemaBlocks = slices.Concat(schemaTopBlocks, kindSchemaBlocks(), schemaNestedBlocks)

// schemaTopBlocks are the blocks that belong to no single proposal kind.
var schemaTopBlocks = []SchemaBlock{
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
		Doc:   "A named GuestOS/replica version, referenced by node and by deploy_guestos as guestos_version.<name>.id. `alp guestos sync` generates the elected set as resources/guestos.hcl.",
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
}

// schemaNestedBlocks are the blocks that appear inside a kind's block rather
// than at the top level, so they have no kinds-table entry of their own.
var schemaNestedBlocks = []SchemaBlock{
	{
		Name: "add / remove", File: "proposals.hcl",
		Doc:   "Inside a membership block: a node to add to or remove from the subnet.",
		Type:  Node{},
		Since: "v0.1.0",
	},
	{
		Name: "features", File: "proposals.hcl",
		Doc:   "Inside an update_subnet block: the subnet's feature flags. Sending it replaces the whole features record rather than merging, so all three fields are required: an omitted one would be written as false. sev_enabled must therefore restate what the subnet already has; the registry rejects a change to it.",
		Type:  subnetFeaturesBody{},
		Since: "v0.5.0",
	},
	{
		Name: "resource_limits", File: "proposals.hcl",
		Doc:   "Inside an update_subnet block: the subnet's state-size limits. Replaced wholesale like features, so both fields are required.",
		Type:  resourceLimitsBody{},
		Since: "v0.5.0",
	},
	{
		Name: "dc", File: "proposals.hcl",
		Doc:   "Inside an add_data_centers block: one data center record to register. Repeat the block for each one. Distinct from the data_center resource block, which only names an existing id for reference; this one carries the fields the registry stores.",
		Type:  dataCenterBody{},
		Since: "v0.5.0",
	},
	{
		Name: "gps", File: "proposals.hcl",
		Doc:   "Inside a dc block: the site's coordinates. Optional, and omitted leaves the record without them.",
		Type:  gpsBody{},
		Since: "v0.5.0",
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

	"NodeProvider.decommissioned": "Marks a node provider removed on-chain. Its block is kept so historical proposal payloads keep resolving to the ids they were submitted with; reconcile expects it, and every operator under it, to be absent from the registry.",

	"NodeOperator.id":       "Principal of the node operator.",
	"NodeOperator.label":    "Human-readable name.",
	"NodeOperator.provider": "Id of its node provider (node_provider.<name>.id).",
	"NodeOperator.dc":       "Id of its data center (data_center.<name>.id).",

	"NodeOperator.decommissioned": "Marks a node operator removed on-chain. Its block is kept so historical proposal payloads keep resolving to the ids they were submitted with; reconcile expects it to be absent from the registry. Implied for every operator under a decommissioned node_provider.",

	"GuestosVersionRes.id":      "GuestOS/replica version hash. Spelled id, not hash, so it resolves through the same <kind>.<name>.id form as every other resource.",
	"GuestosVersionRes.label":   "Human-readable name (e.g. the release name).",
	"GuestosVersionRes.default": "Marks the version every node is expected to run unless its own block overrides it, so a fleet-wide rollout is one edit here rather than one per node. At most one guestos_version may carry it; two is a load error. With a default declared, a node omitting guestos_version inherits it instead of going unchecked, so `guestos_version = \"none\"` is how a single node opts out.",

	"NodeRes.id":              "Principal of the node.",
	"NodeRes.label":           "Human-readable name.",
	"NodeRes.subnet":          "Id of the subnet it belongs to (subnet.<name>.id). Empty means unassigned.",
	"NodeRes.operator":        "Id of its node operator (node_operator.<name>.id).",
	"NodeRes.decommissioned":  "Marks a node deregistered on-chain. Its block is kept so historical proposal payloads keep resolving to the ids they were submitted with; reconcile expects it to be absent from the registry.",
	"NodeRes.guestos_version": "GuestOS/replica version this node is expected to run. Not a registry fact: the registry stores one version per subnet, so reconcile reads the node's own /api/v2/status impl_version, which needs IPv6. If the node is unreachable it falls back to the public dashboard, marking the row \"via dashboard\" since that data may lag. Reconcile also checks the version against the NNS elected set and marks it \"NOT ELECTED\" if absent; when that source is unreadable the check is skipped rather than failing. Omitted inherits the guestos_version marked default, and means unchecked when there is none; setting it while a default exists marks the row \"[override]\". \"none\" opts the node out of the check entirely.",
	"NodeRes.chip_id":         "The node's AMD SEV-SNP CHIP_ID as hex, the form AMD's KDS takes as its hwID parameter (base64 is also accepted; comparison is on the decoded bytes). 64 opaque bytes identifying the physical CPU, recorded by the registry canister only for a node onboarded with SEV. Pins hardware identity, not live TEE state. Reconcile also asks AMD KDS to vouch for the chip, caching the verdict in state.json. Omitted asserts the node carries none, so gaining, losing, or changing one is drift; an unread record reports \"unknown\".",

	"Provider.host":           "Governance host URL. Defaults per command; overridden by --host.",
	"Provider.neuron":         "Proposer neuron id.",
	"Provider.fetch_root_key": "Whether to fetch the IC root key. Unset defaults to true for non-mainnet hosts.",

	"Spec.kind":    "Proposal kind; selects the nested block (membership, deploy_guestos, update_subnet, remove_nodes, remove_node_operators, remove_node_provider, add_data_centers, add_node_provider, add_node_operator).",
	"Spec.title":   "Proposal title shown on the NNS. Governance requires 5 to 256 bytes.",
	"Spec.summary": "Proposal summary (markdown), up to 30000 bytes. A heredoc body is submitted in full, and the dry run prints it back as the NNS stored it.",
	"Spec.url":     "Reference URL for the proposal, such as a forum thread. Governance accepts only an https link on forum.dfinity.org. Alpage applies the same rule when the config loads.",

	"membershipBody.subnet_id": "Subnet whose membership changes (subnet.<name>.id).",
	"membershipBody.add":       "A node to add to the subnet; repeatable. See the add / remove block.",
	"membershipBody.remove":    "A node to remove from the subnet; repeatable. See the add / remove block.",

	"deployGuestosBody.subnet_id":          "Subnet whose nodes to upgrade (subnet.<name>.id).",
	"deployGuestosBody.replica_version_id": "Replica version to deploy to every node in the subnet (guestos_version.<name>.id, or the hash itself). Must be elected by the NNS: preflight checks the registry for a replica_version_<id> record (read via the registry explorer) and refuses an unelected version, since the NNS would reject the proposal. --force submits anyway, and also lets apply proceed when that lookup fails (downgraded to a warning); plan always degrades that way. Preflight additionally resolves the version's release name and election proposal from the public dashboard, which is display-only and degrades to a note when unavailable.",

	"Node.id":    "Node id (node.<name>.id).",
	"Node.label": "Optional human-readable name.",

	"removeNodesBody.node":                    "A node to deregister. Repeat the block for each one.",
	"removeNodeOperatorsBody.operators":       "Ids of the node operators whose records to delete (node_operator.<name>.id).",
	"removeNodeProviderBody.node_provider_id": "Principal of the node provider to remove (node_provider.<name>.id).",

	"addDataCentersBody.dc": "A data center record to register, repeated once per data center. See the dc block.",

	"dataCenterBody.id":     "Registry data center id (data_center.<name>.id), at most 255 bytes and short by convention, e.g. zh3. The canister lowercases it, so alpage does too and the payload matches the record that lands.",
	"dataCenterBody.region": "Registry region string as comma-separated continent,country,city (e.g. Europe,CH,Zurich). Reference data_center.<name>.region so the resource block stays the single source. The node reward table is keyed by region prefix, so this decides what the data center's nodes can earn.",
	"dataCenterBody.owner":  "Legal owner of the data center as the self-declaration names it. Free text the NNS displays, at most 255 bytes.",
	"dataCenterBody.gps":    "Optional coordinates for the site, described by the gps block. Omitting the block leaves the record without them.",

	"gpsBody.latitude":  "Latitude in decimal degrees, between -90 and 90.",
	"gpsBody.longitude": "Longitude in decimal degrees, between -180 and 180.",

	"addNodeProviderBody.node_provider_id": "Principal of the node provider to register (node_provider.<name>.id). Governance rejects a principal it already knows.",
	"addNodeProviderBody.reward_account":   "Ledger account identifier rewards are paid to, as 64 hex characters. Optional: governance pays a provider with no reward_account into the default account of the provider principal, and a later UpdateNodeProvider can redirect the rewards. Governance validates the embedded checksum, so alpage rejects a malformed one when the config loads.",

	"addNodeOperatorBody.node_operator_id":     "Principal of the node operator to register (node_operator.<name>.id). The operator signs add_node with this identity, so it must be a principal they hold the key for.",
	"addNodeOperatorBody.node_provider_id":     "Principal of the provider this operator belongs to (node_provider.<name>.id). Governance rejects the proposal at submission when the provider is not registered, so the add_node_provider proposal has to land first.",
	"addNodeOperatorBody.dc_id":                "Data center the operator's nodes sit in (data_center.<name>.id). The node-operator invariant rejects an unknown one when the proposal executes, so the add_data_centers proposal has to land first.",
	"addNodeOperatorBody.node_allowance":       "How many nodes the operator may register before another proposal raises it. Omitted means 0, which is expected for a Cloud Engine operator because it needs no allowance. Any other operator with a 0 allowance gets a working record that cannot yet add nodes, so preflight reports the value either way.",
	"addNodeOperatorBody.ipv6":                 "The operator's IPv6 prefix, recorded on the operator record. Omitting it leaves the field unset.",
	"addNodeOperatorBody.max_rewardable_nodes": "Node reward type mapped to the maximum number of nodes of that type the provider may be rewarded for, e.g. { type3.1 = 28 }. Omitted means none of them, which is the case for an operator outside the node provider reward scheme. The payload's other reward map, rewardable_nodes, is the field the registry canister itself calls the old one, so alpage always sends it empty rather than exposing it.",

	"updateSubnetBody.subnet_id":               "Subnet whose configuration changes (subnet.<name>.id).",
	"updateSubnetBody.is_halted":               "Whether the subnet is halted: a halted subnet no longer creates or executes blocks. This is how a subnet is frozen for a recovery, and setting it back to false is how it resumes.",
	"updateSubnetBody.halt_at_cup_height":      "Halt the subnet once it reaches the next CUP height rather than immediately, so it stops at a known state. The registry resets this flag when a new CUP proposal is approved and sets is_halted instead, so the subnet stays halted until a later proposal sets is_halted = false.",
	"updateSubnetBody.max_number_of_canisters": "Maximum number of canisters that may be present on the subnet at any one time. 0 does not mean unlimited: it means the replica's own default applies.",
	"updateSubnetBody.ssh_readonly_access":     "Public keys whose owners get \"readonly\" SSH access to every replica on the subnet, for when a subnet recovery has to be performed. The list is replaced, not merged, so it must name every key that keeps access; an empty list revokes all of them. Preflight flags a declared list that clears existing entries.",
	"updateSubnetBody.ssh_backup_access":       "Public keys whose owners get \"backup\" SSH access to the subnet's nodes, which is what makes backing up the NNS possible. Replaced wholesale like ssh_readonly_access.",
	"updateSubnetBody.subnet_admins":           "Principals that have admin privileges on the subnet. Replaced wholesale, so the list must be complete.",
	"updateSubnetBody.features":                "The subnet's feature flags. Replaced wholesale; see the features block.",
	"updateSubnetBody.resource_limits":         "The subnet's resource-consumption limits. Replaced wholesale; see the resource_limits block.",

	"updateSubnetBody.unit_delay_millis":              "Unit delay for the blockmaker, in milliseconds.",
	"updateSubnetBody.initial_notary_delay_millis":    "Initial delay for the notary, in milliseconds, which gives rank-0 blocks time to propagate.",
	"updateSubnetBody.dkg_interval_length":            "Length of every DKG interval, counted in the rounds that follow the DKG summary.",
	"updateSubnetBody.dkg_dealings_per_block":         "Upper bound on the number of DKG dealings allowed in one block.",
	"updateSubnetBody.max_ingress_bytes_per_message":  "Maximum bytes per ingress message. A hard cap: a message over the limit is dropped.",
	"updateSubnetBody.max_ingress_bytes_per_block":    "How big an ingress payload may be when held in memory, which may exceed max_block_payload_size since ingress messages are stripped before a block is disseminated. 0 means the replica's built-in default; too high a value drives up replica memory use.",
	"updateSubnetBody.max_ingress_messages_per_block": "Maximum number of ingress messages per block.",
	"updateSubnetBody.max_block_payload_size":         "Maximum size in bytes a block payload may have when sent over the wire. Setting it too high slows block delivery to peers, which can cause forks as higher-rank blocks get proposed meanwhile.",

	"subnetFeaturesBody.canister_sandboxing": "Whether canister execution happens in a sandboxed process. Disabled by default.",
	"subnetFeaturesBody.http_requests":       "Whether canisters on the subnet may perform outbound HTTP(S) requests.",
	"subnetFeaturesBody.sev_enabled":         "Whether the subnet runs with SEV-SNP enabled. Required because the features record is replaced wholesale, which is also the trap: omitting it reads as disabling SEV, and the registry refuses to disable it. Enabling it is allowed, but execution traps unless every node already carries a chip_id and the subnet's GuestOS version has launch measurements. Preflight blocks a disable and warns on an enable.",

	"resourceLimitsBody.maximum_state_size":  "Maximum subnet state size in bytes.",
	"resourceLimitsBody.maximum_state_delta": "Maximum subnet state delta in bytes.",
}

// SchemaUnreleased is the version Since values carry before that version is
// tagged; the docs list it as unreleased rather than linking a tag that does
// not exist yet. Empty once that version ships, so every entry links its tag.
// Set it to the next version when a change first lands after a release.
const SchemaUnreleased = "v0.5.0"

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

	"NodeProvider.id":             "v0.1.0",
	"NodeProvider.label":          "v0.1.0",
	"NodeProvider.decommissioned": "v0.5.0",

	"NodeOperator.id":             "v0.1.0",
	"NodeOperator.label":          "v0.1.0",
	"NodeOperator.provider":       "v0.1.0",
	"NodeOperator.dc":             "v0.1.0",
	"NodeOperator.decommissioned": "v0.5.0",

	"GuestosVersionRes.id":      "v0.3.0",
	"GuestosVersionRes.label":   "v0.3.0",
	"GuestosVersionRes.default": "v0.5.0",

	"NodeRes.id":              "v0.1.0",
	"NodeRes.label":           "v0.1.0",
	"NodeRes.subnet":          "v0.1.0",
	"NodeRes.operator":        "v0.1.0",
	"NodeRes.decommissioned":  "v0.1.1",
	"NodeRes.guestos_version": "v0.2.0",
	"NodeRes.chip_id":         "v0.4.0",

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

	"removeNodesBody.node":                    "v0.5.0",
	"removeNodeOperatorsBody.operators":       "v0.5.0",
	"removeNodeProviderBody.node_provider_id": "v0.5.0",

	"updateSubnetBody.subnet_id":               "v0.5.0",
	"updateSubnetBody.is_halted":               "v0.5.0",
	"updateSubnetBody.halt_at_cup_height":      "v0.5.0",
	"updateSubnetBody.max_number_of_canisters": "v0.5.0",
	"updateSubnetBody.ssh_readonly_access":     "v0.5.0",
	"updateSubnetBody.ssh_backup_access":       "v0.5.0",
	"updateSubnetBody.subnet_admins":           "v0.5.0",
	"updateSubnetBody.features":                "v0.5.0",
	"updateSubnetBody.resource_limits":         "v0.5.0",

	"updateSubnetBody.unit_delay_millis":              "v0.5.0",
	"updateSubnetBody.initial_notary_delay_millis":    "v0.5.0",
	"updateSubnetBody.dkg_interval_length":            "v0.5.0",
	"updateSubnetBody.dkg_dealings_per_block":         "v0.5.0",
	"updateSubnetBody.max_ingress_bytes_per_message":  "v0.5.0",
	"updateSubnetBody.max_ingress_bytes_per_block":    "v0.5.0",
	"updateSubnetBody.max_ingress_messages_per_block": "v0.5.0",
	"updateSubnetBody.max_block_payload_size":         "v0.5.0",

	"subnetFeaturesBody.canister_sandboxing": "v0.5.0",
	"subnetFeaturesBody.http_requests":       "v0.5.0",
	"subnetFeaturesBody.sev_enabled":         "v0.5.0",

	"resourceLimitsBody.maximum_state_size":  "v0.5.0",
	"resourceLimitsBody.maximum_state_delta": "v0.5.0",

	"addDataCentersBody.dc": "v0.5.0",

	"dataCenterBody.id":     "v0.5.0",
	"dataCenterBody.region": "v0.5.0",
	"dataCenterBody.owner":  "v0.5.0",
	"dataCenterBody.gps":    "v0.5.0",

	"gpsBody.latitude":  "v0.5.0",
	"gpsBody.longitude": "v0.5.0",

	"addNodeProviderBody.node_provider_id": "v0.5.0",
	"addNodeProviderBody.reward_account":   "v0.5.0",

	"addNodeOperatorBody.node_operator_id":     "v0.5.0",
	"addNodeOperatorBody.node_provider_id":     "v0.5.0",
	"addNodeOperatorBody.dc_id":                "v0.5.0",
	"addNodeOperatorBody.node_allowance":       "v0.5.0",
	"addNodeOperatorBody.ipv6":                 "v0.5.0",
	"addNodeOperatorBody.max_rewardable_nodes": "v0.5.0",
}
