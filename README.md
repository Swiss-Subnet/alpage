# Alpage

Declarative node-fleet management for the Internet Computer. Named after the Swiss alpine pasture: you move the herd onto the *alp* and off it as conditions change, the same way nodes join and leave a subnet.

`alpage` manages the fleet the way Terraform manages infrastructure: you declare the desired state in committed config, and the tool submits the NNS proposals that realise it, recording the outcome (which proposal id, submitted by whom, when) in committed state. Every change is verified on a local PocketIC NNS before it can touch the live network.

The CLI is `alp`, built from `cmd/alp`.

## How it works

- **Config** you declare: `proposals.hcl` (one `proposal "<name>"` block per change) and `resources.hcl` (reusable `node` / `subnet` resources, referenced as `node.<name>.id` / `subnet.<name>.id`).
- **State** the tool records: `state.json`, a single file keyed by proposal name, holding the resulting `proposal_id`, the submitter principal, neuron, host, payload hash, and timestamp.
- **Verification**: `apply` always dry-runs the exact payload on a local PocketIC NNS first. It refuses to resubmit a proposal already recorded in state unless `--force`.

## Usage

```
alp apply  <name> --identity <key.pem> [--neuron id] [--host url] [--yes] [--force]
alp import <name> <proposal_id> --identity <key.pem> [--at RFC3339]
alp list
alp registry subnet <subnet_id> [--host url]
```

- `apply` — dry-run on PocketIC, then submit to the live network with `--yes` (type `submit` to confirm), then write state.
- `import` — adopt an already-submitted proposal into state without submitting (Terraform's `import`).
- `list` — show each declared proposal as in-sync, drifted, or not-submitted. Here "drift" means the committed config's payload no longer matches the payload recorded in state (an edit since submit); it does not reconcile against on-chain state.
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

- `resize` — `change_subnet_membership`: add/remove nodes on a subnet.
- `deploy_guestos` — `deploy_guestos_to_all_subnet_nodes`: upgrade every node in a subnet to a replica version.

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

## License

All rights reserved. No license is granted at this time; a license is to be determined. Do not use, copy, distribute, or modify this code without permission.
