# Resources for the golden dry-run fixture. Dummy (synthetic) principals, not
# real Swiss Subnet ids. Self-contained: editing the repo root resources.hcl
# must not affect these tests.
subnet "test" {
  id    = "wmzac-nabae-aqcai-baeaq-caiba-eaqca-ibaea-qcaib-aeaqc-aibae-aqc"
  label = "Test Subnet"
}

node "n1" {
  id    = "uduew-qycai-baeaq-caiba-eaqca-ibaea-qcaib-aeaqc-aibae-aqcai-bae"
  label = "Test Node 1"
}

node "n2" {
  id    = "dchi6-uidam-bqgay-dambq-gayda-mbqga-ydamb-qgayd-ambqg-aydam-bqg"
  label = "Test Node 2"
}

node_provider "np1" {
  id    = "dchi6-uidam-bqgay-dambq-gayda-mbqga-ydamb-qgayd-ambqg-aydam-bqg"
  label = "Test Provider 1"
}

data_center "dc1" {
  id = "dc1"
}

node_operator "op1" {
  id       = "hsi7b-rl4wt-lum3m-ophfi-oxgx5-u2q7r-ak7ag-nyiik-c4gam-epo2r-3qe"
  provider = node_provider.np1.id
  dc       = data_center.dc1.id
}
