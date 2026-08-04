# Changelog

The release workflow extracts the section matching the tag and publishes it as the release notes, so every release needs a section here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Before 1.0, a minor bump may change behaviour.

## [Unreleased]

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
