# Alpage

Declarative node-fleet management for the Internet Computer. Named after the Swiss alpine pasture: you move the herd onto the *alp* and off it as conditions change, the same way nodes join and leave a subnet.

`alpage` manages the fleet the way Terraform manages infrastructure: you declare the desired state in committed config, and the tool submits the NNS proposals that realise it, recording the outcome (which proposal id, submitted by whom, when) in committed state. Every change is verified on a local PocketIC NNS before it can touch the live network.

The CLI is `alp`, built from `cmd/alp`.

## How it works

- **Config** you declare: `proposals.hcl` (one `proposal "<name>"` block per change) and `resources.hcl` (reusable `node` / `subnet` resources, referenced as `node.<name>.id` / `subnet.<name>.id`). Every block and field is documented in [docs/config.md](docs/config.md), generated from the struct tags via `go generate ./nns`. Split resource files: any `resources/*.hcl` beside `resources.hcl` is merged in, so cross-file references resolve.
- **State** the tool records: `state.json`, a single file keyed by proposal name, holding the resulting `proposal_id`, the submitter principal, neuron, host, payload hash, and timestamp.
- **Verification**: `apply` always dry-runs the exact payload on a local PocketIC NNS first. The local registry is seeded with the real records the payload names, copied from the live network at one pinned registry version, so the proposal executes against the same entities mainnet will resolve rather than against empty state. It refuses to resubmit a proposal already recorded in state unless `--force`.

## Usage

```
alp apply  <name> --identity <key.pem> [--neuron id] [--host url] [--yes] [--force]
alp import <name> <proposal_id> --identity <key.pem> [--at RFC3339]
alp list
alp status [--host url] [--refresh]
alp reconcile [--host url]
alp registry subnet <subnet_id> [--host url]
alp guestos sync [--set alias] [--variant base] [--out path]
```

- `apply` — check the `--identity` can submit from the neuron (fails fast on a wrong key), dry-run on PocketIC, then submit to the live network with `--yes` (type `submit` to confirm), then write state.
- `import` — adopt an already-submitted proposal into state without submitting (Terraform's `import`).
- `list` — show each declared proposal as in-sync, drifted, or not-submitted. Here "drift" means the committed config's payload no longer matches the payload recorded in state (an edit since submit); it does not reconcile against on-chain state.
- `status` — read each recorded proposal back from live governance and show its actual on-chain status (open/adopted/rejected/executed/failed) with the current tally. This is the on-chain counterpart to `list`: `list` compares config against state, `status` compares state against the network. Governance purges older proposals; when a recorded id is no longer in governance, `status` falls back to the read-only ICP dashboard API and marks the line `[via ICP dashboard; purged from governance]`. A proposal in neither source is reported as unknown. Read-only. A proposal that has settled (executed, rejected, or failed) is reported from state rather than queried again, since its result cannot change, so a run costs one query per proposal still in flight rather than one per proposal ever submitted. `--refresh` queries everything.
- `reconcile` — read-only diff of `resources.hcl` against live on-chain state: nodes vs subnet membership (`in-sync`/`missing`/`deregistered`/`unmanaged`), `data_center`s vs their registry records (`ok`/`missing`/`region-mismatch`), `node_operator`s vs their provider's on-chain ownership, and each node's declared operator vs its on-chain owner (an operator-owned node missing from `resources.hcl` is reported `unmanaged`). Exits nonzero on drift, so it works as a CI gate. Status fields are colorized on a TTY (honors `NO_COLOR`). Every drift row ends in `fix: ...`, naming the proposal kind or the config edit that resolves it; rows that are in sync carry none.
- `guestos sync` — regenerate `resources/guestos.hcl`, the catalogue of GuestOS versions the NNS currently has elected, so a new release never has to be transcribed by hand. The elected set is replayed from the dashboard's election proposals (elections and the unelections they carry). `--set <alias>` also repoints an existing `guestos_version` block at the newest elected release, editing it in place so its comments and its `default` marker survive and the new hash lands in the diff. `--variant` picks the release flavour (default `base`), since DFINITY elects several per release.
- `registry subnet` — read-only query of the registry canister for a subnet's current node membership, emitted as a `resources.hcl` fragment (a `subnet` block plus one `node` block per member, sorted by id). Use it to seed or reconcile `resources.hcl` against on-chain state.

For example, the current membership of the Swiss subnet (truncated):

```
$ alp registry subnet 3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe
subnet "subnet_3zsyy" {
  id = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"
}

node "node_3wbrf" {
  id = "3wbrf-zokqb-6euxi-6lxxo-i5tia-4742s-7jfsj-touui-qwzbm-7rmdw-nae"
}

node "node_au6oc" {
  id = "au6oc-imc3w-ssdnk-lzy6e-6fgeh-ejwch-bqohf-vj624-k5xfl-77rpz-xqe"
}
```

Block labels are derived placeholders (`subnet_<prefix>` / `node_<prefix>`); rename them and add a `label` by hand, since the registry carries no human names.

Global submission settings (`host`, `neuron`, `fetch_root_key`) live in a `provider {}` block in `proposals.hcl`; CLI flags override them. All three subcommands accept `--config` and `--state` to point at non-default file paths.

## Proposal kinds

Each proposal has a `kind` and a matching nested block:

- `membership` — `change_subnet_membership`: add and/or remove nodes on a subnet. Adding and removing in one proposal swaps a node, leaving the subnet size unchanged.
- `deploy_guestos` — `deploy_guestos_to_all_subnet_nodes`: upgrade every node in a subnet to a replica version.
- `update_subnet` — `update_subnet`: change a subnet's configuration record (halt/unhalt, SSH access, canister cap, features, consensus tuning). Every field is optional and an omitted one is left untouched, but a nested block is replaced wholesale, so `features` and `resource_limits` must be stated in full.
- `remove_nodes` — `remove_nodes`: deregister nodes, freeing their operators' allowance.
- `remove_node_operators` — `remove_node_operators`: delete node operator records.
- `remove_node_provider` — a native `AddOrRemoveNodeProvider` governance action rather than an `ExecuteNnsFunction`: remove a node provider.
- `add_data_centers` — `add_or_remove_data_centers`: register data center records. Adds only, since removal has preconditions of its own.
- `add_node_provider` — the `ToAdd` half of `AddOrRemoveNodeProvider`, also a native governance action: register a node provider.
- `add_node_operator` — `add_node_operator`, submitted as `ASSIGN_NOID`: register a node operator under a provider, in a data center, with an allowance of nodes it may register.

The last three are what onboarding a node provider takes. The data center and the provider are independent of each other, so those two can go in either order or at once, and only the operator depends on both. Nothing in the payloads encodes that dependency, so the operator's preflight checks that its data center and its provider both landed. Registering the nodes themselves is not a proposal at all, since the operator calls `add_node` against the registry directly, spending the allowance the operator proposal granted.

Adding a kind is a new `Action` implementation plus one decode case; state, dry-run, submit, and drift detection are kind-agnostic.

## Development

The toolchain (Go, PocketIC, IC canisters) is pinned in the Nix flake. Run everything through the dev shell:

```
nix develop --command go build ./...
nix develop --command go test ./...
```

Golden dry-run tests regenerate with `-update`:

```
nix develop --command go test ./nns -run TestDryRunGolden -update
```

Build the CLI reproducibly with the flake (version taken from the git rev/tag, injected into `alp version`):

```
nix build .#alp && ./result/bin/alp version
```

## Releases

Tags matching `v*` trigger `.github/workflows/release.yml`, which cross-compiles `alp` for linux/darwin on amd64/arm64 with the tag injected into `main.version`, and attaches the binaries (plus `.sha256` sums) to a GitHub release. To cut one:

```
git tag v0.1.0
git push origin v0.1.0
```

`alp version` on any build reports its tag; a plain `go build` reports `dev` (or the module pseudo-version for a `go install`ed build).

## License

Copyright (c) 2026 Swiss Subnet AG, Zug, Switzerland. Licensed under the [Business Source License 1.1](LICENSE).

Running Alpage against your own fleet of up to 5 nodes is free, including in production, and evaluation is always free at any fleet size. Beyond 5 nodes, managing fleets for third parties, or offering a hosted service built on Alpage needs a commercial license. Each released version converts to Apache-2.0 four years after it is published.

See [docs/licensing.md](docs/licensing.md) for the plain-language version, or contact licensing@subnet.ch. Contributions are accepted under the DCO; see [CONTRIBUTING.md](CONTRIBUTING.md).
