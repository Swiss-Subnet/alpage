# Changelog

The release workflow extracts the section matching the tag and publishes it as the release notes, so every release needs a section here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Before 1.0, a minor bump may change behaviour.

## [Unreleased]

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
