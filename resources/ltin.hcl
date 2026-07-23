node_operator "ltin_fl1" {
  id       = "ziab5-kch42-6jhxt-26xf7-wej5v-xw4oh-36m5y-yba7v-xrtpv-pobv3-fqe"
  provider = node_provider.ltin.id
  dc       = data_center.fl1.id
}

node "ltin_eschen_1" {
  id       = "rejp3-jtsp3-hyujo-jrg2d-aywhf-j7w75-ndvb7-orub2-km7uc-4yuox-cqe"
  label    = "LTIN AG, Eschen 1"
  operator = node_operator.ltin_fl1.id
  subnet   = subnet.swiss.id
}
