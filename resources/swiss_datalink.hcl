node_operator "swiss_datalink_ag1" {
  id       = "yedtm-rm5av-s256v-zzi4w-7lxen-koqg6-pzak3-rjzko-xfu2c-dw7eo-bae"
  provider = node_provider.swiss_datalink.id
  dc       = data_center.ag1.id
}

node "swiss_datalink_aargau_1" {
  id       = "eaaef-crr36-d3ou2-fuyyz-mluny-cfm3r-zu4rf-22dhz-njgck-jres2-xqe"
  label    = "Swiss Datalink AG, Aargau 1"
  operator = node_operator.swiss_datalink_ag1.id
  subnet   = subnet.swiss.id
}
