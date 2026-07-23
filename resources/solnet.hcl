node_operator "solnet_so1" {
  id       = "nwpoe-kxdae-5afgc-itoe7-xecg7-k74et-jggwc-lgz3n-tjs6e-mtqos-5ae"
  provider = node_provider.solnet.id
  dc       = data_center.so1.id
}

node "solnet_solothurn_1" {
  id       = "3wbrf-zokqb-6euxi-6lxxo-i5tia-4742s-7jfsj-touui-qwzbm-7rmdw-nae"
  label    = "SolNet, Solothurn 1"
  operator = node_operator.solnet_so1.id
  subnet   = subnet.swiss.id
}
