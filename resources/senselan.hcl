node_operator "senselan_ne1" {
  id       = "2mie5-aobqb-l2gvy-2or7x-di7cx-3xoij-ifiej-zqxlm-hzn6s-lp2ih-kqe"
  provider = node_provider.senselan.id
  dc       = data_center.ne1.id
}

node "senselan_fribourg_1" {
  id       = "mrpo5-7xhsn-22itc-szjxm-2e6lm-55mwi-xysfj-razxo-blba5-zqerk-3qe"
  label    = "senseLAN, Fribourg 1"
  operator = node_operator.senselan_ne1.id
  subnet   = subnet.swiss.id
}
