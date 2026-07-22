# NNS proposals for the Swiss Subnet. Each block is one proposal; results are
# recorded in state.json keyed by the block label.

provider {
  host   = "https://icp-api.io"
  neuron = 12838523358913392196
  # fetch_root_key: leave unset for mainnet; the tool sets it true for non-mainnet hosts.
}

proposal "swiss-subnet-wave1" {
  kind    = "resize"
  title   = "Resize the Swiss Subnet for the SEV migration (wave 1)"
  summary = "Remove 6 nodes (13 -> 7) to be redeployed with SEV-SNP enabled as the first wave of the confidential-computing migration."
  url     = "https://forum.dfinity.org/t/migrating-the-swiss-subnet-to-confidential-computing-sev-snp/74619"

  resize {
    subnet_id = subnet.swiss.id

    remove { id = node.achermann_luzern_1.id }
    remove { id = node.coreledger_zug_1.id }
    remove { id = node.alpinedc_vaud_1.id }
    remove { id = node.big_graubunden_1.id }
    remove { id = node.noku_ticino_1.id }
    remove { id = node.avalution_appenzell_1.id }
  }
}
