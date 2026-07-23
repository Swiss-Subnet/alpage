node_operator "alpinedc_vd1" {
  id       = "u7afs-z2fqh-zbqyo-jufwe-3vqqs-chc7f-k2fe4-rt66w-l4qia-keuuj-qqe"
  provider = node_provider.alpinedc.id
  dc       = data_center.vd1.id
}

# Original Vaud node g5s3p-... was decommissioned and redeployed with SEV under
# the id below; the old node is deregistered on-chain.
node "alpinedc_vaud_1" {
  id       = "nxhqa-kzj5q-xnggc-skaek-jvtgl-gyl2b-jg6ud-fdxry-ak7fg-onr77-bqe"
  label    = "AlpineDC SA, Vaud 1"
  operator = node_operator.alpinedc_vd1.id
}
