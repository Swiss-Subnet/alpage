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
