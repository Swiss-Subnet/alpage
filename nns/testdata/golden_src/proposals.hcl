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

proposal "remove-nodes-fixture" {
  kind    = "remove_nodes"
  title   = "Remove nodes fixture"
  summary = "Golden fixture: deregister two nodes."
  url     = "https://forum.dfinity.org/t/test-remove-nodes-fixture"

  remove_nodes {
    node { id = node.n1.id }
    node { id = "dchi6-uidam-bqgay-dambq-gayda-mbqga-ydamb-qgayd-ambqg-aydam-bqg" }
  }
}

proposal "remove-node-operators-fixture" {
  kind    = "remove_node_operators"
  title   = "Remove node operators fixture"
  summary = "Golden fixture: delete an operator record."
  url     = "https://forum.dfinity.org/t/test-remove-operators-fixture"

  remove_node_operators {
    operators = [node_operator.op1.id]
  }
}

# The only native governance action here: no ExecuteNnsFunction wrapper and no
# NNS function number, unlike every other kind above.
proposal "remove-node-provider-fixture" {
  kind    = "remove_node_provider"
  title   = "Remove node provider fixture"
  summary = "Golden fixture: remove a node provider."
  url     = "https://forum.dfinity.org/t/test-remove-provider-fixture"

  remove_node_provider {
    node_provider_id = node_provider.np1.id
  }
}
