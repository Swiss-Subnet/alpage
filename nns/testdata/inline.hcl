# Literal-id twin of config.hcl's membership-example; must hash identically.
proposal "membership-example" {
  kind  = "membership"
  title = "Membership fixture"

  membership {
    subnet_id = "67htk-vfkxp-gn33q-baibq"
    remove { id = "5ffj3-jarcq-lruhj-aemtc-sla" }
  }
}
