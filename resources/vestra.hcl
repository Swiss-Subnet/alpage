node_operator "vestra_fl2" {
  id       = "hsi7b-rl4wt-lum3m-ophfi-oxgx5-u2q7r-ak7ag-nyiik-c4gam-epo2r-3qe"
  provider = node_provider.vestra.id
  dc       = data_center.fl2.id
}

node "vestra_vaduz_1" {
  id       = "au6oc-imc3w-ssdnk-lzy6e-6fgeh-ejwch-bqohf-vj624-k5xfl-77rpz-xqe"
  label    = "vestra ICT AG, Vaduz 1"
  operator = node_operator.vestra_fl2.id
  subnet   = subnet.swiss.id
}
