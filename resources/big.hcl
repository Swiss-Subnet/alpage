node_operator "big_gr1" {
  id       = "3pvkg-yll72-7bgau-ifrdj-5hfoz-qurfj-vvl2l-6ztjm-rdbg2-56al6-cqe"
  provider = node_provider.big.id
  dc       = data_center.gr1.id
}

node "big_graubunden_1" {
  id       = "lemsa-bnpvg-zzzcq-6uwar-njtds-byn3n-zcb7v-du25b-tktfx-32gc5-zae"
  label    = "Blockchain Innovation Group, Graubunden 1"
  operator = node_operator.big_gr1.id
}
