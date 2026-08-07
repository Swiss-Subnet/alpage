# Changelog

The release workflow extracts the section matching the tag and publishes it as the release notes, so every release needs a section here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Before 1.0, a minor bump may change behaviour.

## [0.4.0] - 2026-08-07

### Added

- `node` resources accept `chip_id`, the node's AMD SEV-SNP CHIP_ID as the registry records it. Declared as hex, the form AMD's KDS takes as its hwID parameter; the base64 the registry explorer serves is also accepted, since comparison is on the decoded bytes. Omitting it asserts the node carries none, so a node that gains, loses, or changes its chip surfaces as drift; a node whose record cannot be read is reported `unknown` rather than counted as drift.
- `reconcile` asks AMD's Key Distribution Service to vouch for each on-chain chip, confirming the VCEK certificate AMD signs carries that chip id, and reports the silicon stepping it returns. A chip AMD will not vouch for is drift on its own: the config and the registry can agree on a chip that is not genuine. A rate-limited or unreachable lookup is reported as inconclusive rather than counted against the chip. This attests hardware identity, not that a node is currently running attested.
- Verified chips are recorded in `state.json` under `chips`. Silicon does not stop being genuine, so a verdict is kept permanently rather than expiring: only chips never seen before cost a lookup, which keeps repeat runs offline and off KDS's rate limit. Negative and inconclusive verdicts are never cached.
- `reconcile --refresh-chips` re-asks AMD about every chip, ignoring cached verdicts, so a verdict recorded wrongly can be corrected without hand-editing `state.json`. A refusal drops the cached entry; an inconclusive answer leaves it untouched.

### Changed

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
