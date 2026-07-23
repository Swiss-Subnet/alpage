node_operator "coreledger_sz1" {
  id       = "tnqod-r52q6-ub547-756zx-7fokh-l5uga-23zbn-lll2x-ebe5r-fdgyh-oae"
  provider = node_provider.coreledger.id
  dc       = data_center.sz1.id
}

node "coreledger_zug_1" {
  id       = "ezsx4-peoff-6kofj-yz6vt-gc42v-iugvx-vit2r-edy37-qv4bt-ivcxy-kae"
  label    = "CoreLedger, Zug 1"
  operator = node_operator.coreledger_sz1.id
}
