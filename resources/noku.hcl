node_operator "noku_ti1" {
  id       = "7v3fg-puvon-km4rh-gnvqw-pmlug-5iaen-s3v45-kwowl-etrtl-xc245-qqe"
  provider = node_provider.noku.id
  dc       = data_center.ti1.id
}

node "noku_ticino_1" {
  id       = "rumwa-ihvrd-kqzmq-4ainw-birq6-2y4v7-sbqyx-lw3vm-osdhm-nh5hl-cae"
  label    = "NOKU SA, Ticino 1"
  operator = node_operator.noku_ti1.id
}
