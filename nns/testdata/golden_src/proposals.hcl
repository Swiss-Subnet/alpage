# Golden dry-run fixture: one proposal per supported kind, exercising both
# resource references and inline ids. Dummy data, not the repo's production
# config.
proposal "membership-fixture" {
  kind    = "membership"
  title   = "Membership fixture"
  summary = "Golden fixture: remove two nodes."
  url     = "https://forum.dfinity.org/t/test-membership-fixture"

  membership {
    subnet_id = subnet.test.id
    remove { id = node.n1.id }
    remove { id = "dchi6-uidam-bqgay-dambq-gayda-mbqga-ydamb-qgayd-ambqg-aydam-bqg" }
  }
}

proposal "deploy-guestos-fixture" {
  kind    = "deploy_guestos"
  title   = "Deploy GuestOS fixture"
  summary = "Golden fixture: upgrade all subnet nodes."
  url     = "https://forum.dfinity.org/t/test-guestos-fixture"

  deploy_guestos {
    subnet_id          = subnet.test.id
    replica_version_id = "0000000000000000000000000000000000000000"
  }
}
