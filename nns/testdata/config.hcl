# Self-contained fixtures for the spec tests, loaded instead of the repo-root
# config so edits to the live proposals.hcl / resources.hcl cannot break them.
# All ids and names here are anonymous placeholders. inline.hcl describes the
# identical payload with literal ids, proving reference form == inline form.

provider {
  host   = "https://example.invalid"
  neuron = 1
}

proposal "resize-example" {
  kind  = "resize"
  title = "Resize fixture"

  resize {
    subnet_id = subnet.test.id
    remove { id = node.n1.id }
  }
}

proposal "deploy-guestos-example" {
  kind  = "deploy_guestos"
  title = "Deploy GuestOS fixture"

  deploy_guestos {
    subnet_id          = subnet.test.id
    replica_version_id = "0000000000000000000000000000000000000000"
  }
}
