node_operator "achermann_lu1" {
  id       = "rsrz6-sc2oj-azljq-mxw7a-daryz-yxmzm-qyn67-6hls2-vnn5d-izzf5-pqe"
  provider = node_provider.achermann.id
  dc       = data_center.lu1.id
}

node "achermann_luzern_1" {
  id       = "eihqt-opds6-5r7xa-hr32z-k2xoa-bh4l4-65kld-v7gk3-bplt5-vmsw2-mqe"
  label    = "achermann.swiss, Luzern 1"
  operator = node_operator.achermann_lu1.id
}
