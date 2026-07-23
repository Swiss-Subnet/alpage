node_operator "avalution_ai1" {
  id       = "ilwyu-pfcy7-2iy3t-cjmsx-nrw4l-6rmek-lduaa-yha6b-7wck6-3usxt-cqe"
  provider = node_provider.avalution.id
  dc       = data_center.ai1.id
}

node "avalution_appenzell_1" {
  id       = "vou34-3jw7y-l2tah-tssac-y5xop-3x3q4-z2ivi-gvmoc-amv7t-akkbz-vae"
  label    = "Avalution AG, Appenzell Ausserrhoden 1"
  operator = node_operator.avalution_ai1.id
}
