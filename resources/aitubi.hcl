node_operator "aitubi_vs1" {
  id       = "q4gds-li2kf-dhmi6-vmtxg-zrgep-3te7r-2a4ji-nszwv-66biu-dkl6k-eqe"
  provider = node_provider.aitubi.id
  dc       = data_center.vs1.id
}

node "aitubi_valais_1" {
  id       = "ugktj-anayx-3xzbp-hznv5-aznxn-5nodb-4ahcq-7og6o-hzazt-7ziym-zae"
  label    = "Aitubi AG, Valais 1"
  operator = node_operator.aitubi_vs1.id
  subnet   = subnet.swiss.id
}
