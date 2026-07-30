# Licensing

Alpage is published under the [Business Source License 1.1](../LICENSE) (BUSL-1.1). Copyright is held by Swiss Subnet AG, Zug, Switzerland (CHE-343.725.544).

BUSL is source-available, not open source. The source is public, you can read it, build it, fork it, and run it; what is restricted is a narrow set of commercial uses. This page explains the boundary in plain language. The [LICENSE](../LICENSE) file is what governs; this page is guidance, not a contract.

## The short version

Running Alpage against your own fleet of up to 10 nodes is free, including in production. Beyond 10 nodes, or if you manage fleets for other people, you need a commercial licence from Swiss Subnet AG. Evaluating and testing is always free, at any fleet size.

## No licence needed

- Running `alp apply`, `alp status`, `alp reconcile`, or any other subcommand against node machines and subnets you own or operate, up to 10 node machines in total.
- Doing so in production, indefinitely, with no fee and no registration.
- Running Alpage in your own CI as a drift gate (`alp reconcile` exits nonzero on drift).
- Reading, modifying, and forking the source for your own use.
- **Non-production use of any kind, with no node limit**: evaluating, testing, benchmarking, auditing the code before you trust it with governance proposals. Run it against a 40-node fleet in dry-run all you like.
- Letting your own contractors or agents operate Alpage on your behalf, on your fleet, under your direction. Their use counts as yours; hiring help does not itself need a licence.

## Commercial licence needed

- Production use across **more than 10 node machines**. Nodes are counted across your whole organisation, including affiliates under common control, whether or not they are currently assigned to a subnet.
- Providing node or fleet management **as part of your own service offering** to third parties. Operating clients' fleets as a business needs a licence regardless of how many nodes are involved.
- Offering Alpage, or a service built on it, to third parties on a hosted basis: a dashboard, a managed fleet-management product, a monitoring service that submits proposals.
- Embedding Alpage's source or binary in such an offering, or packaging your offering so that Alpage must be fetched for it to work.

### Counting nodes

The limit counts node machines you own or operate, organisation-wide. Unassigned nodes count. Nodes across multiple data centres or subnets all count toward the same total. The number is the one `alp reconcile` sees and is verifiable on-chain, so there is nothing to self-report.

The limit is set per version. If it changes in a later release, versions you already have keep the terms they shipped with.

If you are unsure which side of the line you are on, ask: <licensing@subnet.ch>. Getting a written answer is free and faster than guessing.

## The four-year clock

Every released version converts to the [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0) four years after that version is published. Conversion is automatic and irrevocable: on that date the restrictions above lapse for that version and it becomes fully open source.

The clock runs per version, so `v1.0.0` converts four years after `v1.0.0` shipped, independently of `v2.0.0`. Publication dates are recorded in each version's [GitHub release](https://github.com/swiss-subnet/alpage/releases) notes so the date is auditable rather than a matter of memory.

Practically: the restrictions only ever apply to the recent releases anyone actually wants to run, and nothing can be retroactively withdrawn from you.

## Why not open source

Alpage signs and submits NNS proposals that change live subnet membership and replica versions. Two things follow.

The source must be public. Nobody should trust a tool with that authority as a black box, so evaluation and audit are unrestricted, and every payload is dry-run on a local PocketIC NNS before it can touch the live network.

Development also has to be funded, and the tool needs someone accountable when a proposal has to go out correctly. BUSL keeps the code readable and auditable while reserving the commercial cases that fund the work. It is the same licence Terraform uses, which is fitting for a tool whose model it borrows.

## Commercial licences and support

Contact <licensing@subnet.ch> for:

- A commercial licence covering third-party fleet management or a hosted offering.
- Support with an SLA, for teams that want someone on the hook when a proposal has to land.

## Contributing

Contributions are accepted under the Developer Certificate of Origin 1.1; see [CONTRIBUTING.md](../CONTRIBUTING.md). Contributed code is licensed under BUSL-1.1 along with the rest of the project.

## Third-party components

Alpage links Go modules under the Apache-2.0, MIT, BSD, and MPL-2.0 licences. Their notices and full texts are reproduced in [THIRD_PARTY_LICENSES](../THIRD_PARTY_LICENSES), which is generated from the versions pinned in `go.mod`. None of them restrict the terms above.

HCL is MPL-2.0, which is weak copyleft at file level. Alpage imports it without modification, so the only obligation is the notice already reproduced. If you patch HCL's own source files in a fork, you must publish those file changes.

The Nix flake also fetches DFINITY artifacts that are executed as separate processes rather than linked into `alp`: the `pocket-ic` binary used to dry-run every payload, the governance, registry, and root canister WASMs loaded into it, and the registry `.proto` definitions compiled into `nns/registrypb`. All three are Apache-2.0 at the pinned IC release.

Worth knowing if you fork or vendor from `dfinity/ic` yourself: that repository licenses **per directory**. Most of it is Apache-2.0, but some directories use the Internet Computer Community Source License or the IC Shared Community Source License, which are more restrictive. The artifacts Alpage uses are Apache-2.0 in their own directories, so nothing carries a restriction into Alpage. Re-check when bumping `icReleaseTag` in `flake.nix`, since directory licensing can change between releases.
