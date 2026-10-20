# Changelog

The release workflow extracts the section matching the tag and publishes it as the release notes, so every release needs a section here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Before 1.0, a minor bump may change behaviour.

## [Unreleased]

### Added

- Three removal proposal kinds: `remove_nodes` (deregister nodes), `remove_node_operators` (delete operator records), and `remove_node_provider`. Each preflights the precondition its canister enforces, which otherwise fails late: a node still in a subnet makes `remove_nodes` panic *after* the NNS adopts the proposal, and an operator that still owns nodes is silently dropped from the payload, so the proposal reports success while changing nothing.
- `remove_node_provider` is the first native governance action alpage submits rather than an `ExecuteNnsFunction`. `Action` gained `ProposalAction()` so a kind builds its own governance action; the existing kinds return a shared `ExecuteNnsFunction` helper and are otherwise unchanged.

- Three onboarding proposal kinds, the counterparts of the removal ones: `add_data_centers` (register data center records), `add_node_provider`, and `add_node_operator`. Together they are what it takes to bring a new node provider onto the network. The data center and the provider are independent of each other, so those two can go in either order or at once, and only the operator depends on both. Nothing in the payloads encodes that, so the operator's preflight checks both: an operator naming a data center the registry does not have fails the node-operator invariant *after* adoption, and governance rejects the proposal at submission when the provider is not registered.
- `add_data_centers` preflights the two ways `add_or_remove_data_centers` misbehaves. A `dc_id` the registry already holds makes the canister trap after adoption, even though the mutation it builds is an upsert, and a record whose `id`, `region`, or `owner` exceeds 255 bytes is dropped from the batch with only a log line, so the proposal executes and reports success having registered nothing.
- `node_allowance` defaults to 0, which registers a working operator that cannot yet register nodes. Cloud Engine operators need no allowance, so preflight reports the value rather than treating 0 as a mistake.
- `rewardable_nodes` is deliberately not exposed. The registry canister calls it the old field, and the current reward calculation reads neither it nor `max_rewardable_nodes`, deriving rewardability from each node record's `node_reward_type` and its operator's data center region instead. alpage always sends it empty and exposes only `max_rewardable_nodes`.
- `reward_account` on `add_node_provider` is validated when the config loads. Governance checks the account identifier's embedded CRC32 and rejects a bad one at submission, which is a slow way to find a typo.
- `data_center.<name>.region` now resolves in `proposals.hcl`, so an `add_data_centers` proposal names the region its `data_center` block already declares instead of restating it where the two can drift.
- `alp reconcile` checks `data_center` blocks against their registry records. The region was already compared, but only through an operator whose provider was queried, so a data center with no operator under it went unchecked entirely.
- `alp status` no longer re-queries proposals that have settled. An executed, rejected, or failed proposal cannot change on-chain, so its result is recorded once and reported from state afterwards. The run used to cost one governance query per proposal ever submitted, which only ever grows, and now costs one per proposal still in flight. `state.json` gained an `outcome` per settled entry, written once and never rewritten, so a cached line is identical to a queried one rather than a shortened version of it. `--refresh` queries everything. Entries recorded before this change have no outcome, so they are queried once more and cached after that.
- Every `alp reconcile` drift row now ends in `fix: ...`, naming the proposal or the edit that resolves it. Rows used to state the mismatch and stop, leaving the reader to work out which kind applied. A remedy only names a kind alpage implements, so a `dc-mismatch` says to correct `resources.hcl` rather than pointing at the `update_node_operator_config` alpage has no kind for. `Remedier` is the interface behind it, and `TestEveryDriftStatusHasRemedy` fails on a drift status that names nothing, so a new one cannot ship without advice.

- `alp guestos sync` generates `resources/guestos.hcl` from the GuestOS versions the NNS currently has elected, so the version elected every week or two no longer has to be transcribed by hand. The elected set is replayed from the dashboard's election proposals, applying the unelections each one carries; `blessed_replica_versions` is not maintained and cannot be used for this. `--set <alias>` repoints a `guestos_version` block at the newest elected release, rewriting it in place so its comments and `default` marker survive.
- `guestos_version` blocks accept `default = true`, the version every node is expected to run unless its own block overrides it. A subnet upgrades all at once, so a per-node version is the exception: with a default declared, a release bump is one edit rather than one per node. Two defaults are a load error. Reconcile names the default under the version table and marks the rows that override it.
- `guestos_version.<name>.id` now resolves in `proposals.hcl`, so a `deploy_guestos` proposal names a version the same way nodes do instead of repeating the hash.

- `update_subnet` proposals change a subnet's configuration record: halt and unhalt it, grant SSH access, cap its canister count, restate its features, or retune consensus. Every field is optional and the registry skips an unset one, so a proposal changes exactly what it declares; a nested block is replaced wholesale instead, so `features` and `resource_limits` require all of their fields. Preflight diffs every declared field against the current record, reports the ones already at their declared value, and blocks a `sev_enabled` change, which the registry rejects because SEV can only be set when a subnet is created.

- The dry-run runs against the live network's registry records for the subnet, nodes, operators, and versions the proposal names, so a payload now executes locally instead of failing on state the local NNS never had. Every membership, deploy, and removal dry-run used to end in `Status: Failed` with `Subnet record ... not found in the registry`, which proved the payload decoded and nothing more. A proposal naming an entity the network does not have fails the dry-run with that entity named.

### Changed

- A node omitting `guestos_version` inherits the `guestos_version` marked `default` instead of going unchecked. Without a default declared, omitting it still means unchecked, so an existing config is unaffected until one is added. `guestos_version = "none"` opts a single node out.

- `plan` and a submit's preflight always print what they checked and end on an explicit verdict. A clean result used to print nothing at all, which reads the same as a proposal that was never parsed, so a plan that silently did nothing was indistinguishable from one that passed.

## [0.4.0] - 2026-08-07

### Added

- `node` resources accept `chip_id`, the node's AMD SEV-SNP CHIP_ID as the registry records it. Declared as hex, the form AMD's KDS takes as its hwID parameter; the base64 the registry explorer serves is also accepted, since comparison is on the decoded bytes. Omitting it asserts the node carries none, so a node that gains, loses, or changes its chip surfaces as drift; a node whose record cannot be read is reported `unknown` rather than counted as drift.
- `reconcile` asks AMD's Key Distribution Service to vouch for each on-chain chip, confirming the VCEK certificate AMD signs carries that chip id, and reports the silicon stepping it returns. A chip AMD will not vouch for is drift on its own: the config and the registry can agree on a chip that is not genuine. A rate-limited or unreachable lookup is reported as inconclusive rather than counted against the chip. This attests hardware identity, not that a node is currently running attested.
- Verified chips are recorded in `state.json` under `chips`. Silicon does not stop being genuine, so a verdict is kept permanently rather than expiring: only chips never seen before cost a lookup, which keeps repeat runs offline and off KDS's rate limit. Negative and inconclusive verdicts are never cached.
- `reconcile --refresh-chips` re-asks AMD about every chip, ignoring cached verdicts, so a verdict recorded wrongly can be corrected without hand-editing `state.json`. A refusal drops the cached entry; an inconclusive answer leaves it untouched.

### Changed

- A node declared `decommissioned` is now confirmed against its own registry record rather than inferred from operator node lists. Absence from every declared operator is not proof of deregistration: a node moved to an operator `resources.hcl` does not declare looked identical and passed silently. A decommissioned node the registry still registers is now reported as drift.
- `reconcile` reports drift for a node whose registry record carries a `chip_id` that `resources.hcl` does not declare. An unchanged config that passed before can now exit nonzero, so a fleet with SEV nodes needs its chips declared once; `reconcile` prints each on-chain value to paste in.

### Fixed

- `nix build` and `nix run` stamp the release version into `alp version` again. The version was derived from flake metadata that a tag-pinned flake input does not expose, so a consumer pinning a release tag got the short commit rev instead of the tag. A build off a working tree reports a `-dirty` suffix, so it is never mistaken for the release it is based on.

## [0.3.0] - 2026-08-04

### Added

- `guestos_version` resources name a GuestOS/replica version once, referenced from a node as `guestos_version.<name>.id`, so a fleet-wide rollout is a single edit rather than one per node. Nodes may still declare a literal hash.
- The generated config reference records the release each block and field first appeared in, so a field newer than your binary is visible as such rather than looking like a missing feature.

### Changed

- The `resize` proposal kind is renamed to `membership`, matching the `change_subnet_membership` NNS function it submits. The old name implied the node count always changes, but the common case adds and removes in one proposal to swap a node, leaving the subnet the same size. Rename `kind = "resize"` to `kind = "membership"` and the nested `resize { }` block to `membership { }`; there is no deprecation alias, so the old name is now a load error. The payload encoding is unchanged, so pinned payload hashes and in-flight proposals are unaffected.
- The BUSL Additional Use Grant now covers fleets of up to five node machines, down from ten. Non-production use stays unrestricted at any fleet size.

## [0.2.0] - 2026-07-30

### Added

- `subnet` resources accept `type`, `cost_schedule`, and `admins`, reconciled against the registry subnet record. Read-only half of cloud engine support.
- `ValidateSubnet` runs at load, so a declaration the registry would reject (a `cloud_engine` off the free schedule, admins on a subnet that may not carry them, over 10 admins) fails before a proposal is cut.
- Nodes reconcile their GuestOS version against the NNS elected set.
- Duplicate resource and proposal names are rejected, reporting both sites.

### Fixed

- `deploy_guestos` used NNS function 58 (`SET_DEFAULT_INITIAL_DKG_SUBNET`) instead of 11. Governance accepted it, so such a proposal was adopted and then trapped in the wrong registry method; drift detection also misread stored proposals.
- NNS function numbers now come from the governance protobuf at the pinned release rather than being hand-written.

### Changed

- Pinned IC release moves to `release-2026-07-23_04-21-base`.
- Licensed under BUSL-1.1 (see LICENSE, docs/licensing.md).
- Releases now ship a per-platform `.tar.gz` carrying the binary, licence texts, and docs, alongside the bare binary. Both are checksummed.
