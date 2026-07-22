# NNS proposals for the Swiss Subnet, declared terraform-style. Each block is
# one proposal; state/results are recorded separately in state.json keyed by the
# block label. Edit here, then `go run ./cmd/submit apply <name>`.

# provider is the global submission config (like a terraform provider block).
# CLI flags override these; these override the built-in defaults.
provider {
  host   = "https://icp-api.io"
  neuron = 12838523358913392196
  # fetch_root_key: true only for local/test networks (e.g. PocketIC). Leave
  # unset/false for real mainnet. When unset the tool derives it from the host
  # (true for any host other than mainnet).
  # fetch_root_key = false
}

# Common fields (kind, title, summary, url) are flat; the kind-specific payload
# lives in a nested block named after the kind.
proposal "swiss-subnet-wave1" {
  kind    = "resize"
  title   = "Resize the Swiss Subnet for the SEV migration (wave 1)"
  summary = "Remove 6 nodes (13 -> 7) to be redeployed with SEV-SNP enabled as the first wave of the confidential-computing migration."
  url     = "https://forum.dfinity.org/t/migrating-the-swiss-subnet-to-confidential-computing-sev-snp/74619"

  resize {
    subnet_id = subnet.swiss.id

    # Reference node resources from resources.hcl; inline id = "..." still works.
    remove { id = node.achermann_luzern_1.id }
    remove { id = node.coreledger_zug_1.id }
    remove { id = node.alpinedc_vaud_1.id }
    remove { id = node.big_graubunden_1.id }
    remove { id = node.noku_ticino_1.id }
    remove { id = node.avalution_appenzell_1.id }
  }
}

# Example of a second proposal kind (not yet submitted). Upgrades every node in
# the subnet to a replica (guest OS) version.
proposal "swiss-subnet-guestos-example" {
  kind    = "deploy_guestos"
  title   = "Deploy GuestOS to all Swiss Subnet nodes (example)"
  summary = "Upgrade every node in the subnet to the given replica version. Example block; replace replica_version_id before submitting."
  url     = "https://forum.dfinity.org/t/migrating-the-swiss-subnet-to-confidential-computing-sev-snp/74619"

  deploy_guestos {
    subnet_id          = subnet.swiss.id
    replica_version_id = "0000000000000000000000000000000000000000"
  }
}
