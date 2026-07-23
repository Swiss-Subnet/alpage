node_operator "decentralized_fl3" {
  id       = "a7uqi-wzcgk-bjijx-f562s-a2pm6-qriyn-qmvcw-jtzrw-nx64c-3ocff-iqe"
  provider = node_provider.decentralized.id
  dc       = data_center.fl3.id
}

node "decentralized_schaan_1" {
  id       = "rhzxx-ciqqi-eamo5-ntkpb-rx3uq-nmlih-olmgl-o6uld-dafwi-ahgiu-3ae"
  label    = "Decentralized, Schaan 1"
  operator = node_operator.decentralized_fl3.id
  subnet   = subnet.swiss.id
}
